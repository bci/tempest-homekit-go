// Package editor implements the standalone alarm-editor web UI: a small HTTP
// server that serves a single-page editor (indexHTML plus static CSS/JS) and a
// JSON API for reading, validating and saving an alarm configuration file, and
// for managing the CONTACT_LIST and TAG_LIST values used by the editor's
// dropdowns.
//
// The server keeps the configuration in memory and rewrites the whole file on
// every successful mutation; it does not watch the file for external edits.
package editor

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"tempest-homekit-go/pkg/alarm"
	"tempest-homekit-go/pkg/logger"
	"tempest-homekit-go/pkg/weather"
)

// Server represents the alarm editor web server.
//
// A Server holds the alarm configuration in memory and persists it to
// configPath on every mutating API call. Its fields are not guarded by a
// mutex, so concurrent requests share state without synchronisation.
//
// Fields:
//   - configPath is the alarm config JSON file being edited, with any
//     leading "@" already stripped.
//   - port is the TCP port (without colon) that Start listens on.
//   - version is the application version shown in the editor page.
//   - envFile is an optional .env-style file from which CONTACT_LIST and
//     TAG_LIST are read and to which they are saved; when empty the process
//     environment (for reads) and ".env"/".env.example" (for saves) are used.
//   - config is the in-memory alarm configuration served and mutated by the API.
//   - lastLoadTime is when config was last read from disk; zero if it was
//     created fresh because the file did not exist.
//   - contacts is the contact list offered in the editor's recipient dropdowns.
type Server struct {
	configPath   string
	port         string
	version      string
	envFile      string
	config       *alarm.AlarmConfig
	lastLoadTime time.Time
	contacts     []Contact
}

// Contact represents a contact entry for alarm notifications.
//
// Contacts are stored as a JSON array in the CONTACT_LIST variable (or in
// contacts.json when saved that way).
//
// Fields:
//   - Name is the display name; required when saving.
//   - Email is the contact's email address; must contain "@" if set.
//   - SMS is the contact's phone number in international form; must start
//     with "+" if set. At least one of Email or SMS is required when saving.
type Contact struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	SMS   string `json:"sms"`
}

// NewServer creates a new alarm editor server.
//
// configPath is the alarm configuration file to edit; a leading "@" (the
// command-line file-reference syntax) is stripped. port is the listen port
// without a colon, version is shown in the UI, and envFile optionally names
// a .env file to read and write CONTACT_LIST/TAG_LIST instead of using the
// process environment.
//
// A failure to load contacts is logged and ignored. If the config file does
// not exist, an empty configuration is used and the file is created on the
// first save. Any other read or parse error is returned.
func NewServer(configPath, port, version, envFile string) (*Server, error) {
	// Remove @ prefix if present
	path := strings.TrimPrefix(configPath, "@")

	s := &Server{
		configPath: path,
		port:       port,
		version:    version,
		envFile:    envFile,
	}

	// Load contact list from environment
	if err := s.loadContacts(); err != nil {
		logger.Warn("Failed to load contact list: %v", err)
		// Continue with empty contact list
	}

	// Load existing config or create new one
	if err := s.loadConfig(); err != nil {
		// If file doesn't exist, create a minimal config
		if os.IsNotExist(err) {
			s.config = &alarm.AlarmConfig{
				Alarms: []alarm.Alarm{},
			}
			logger.Info("Creating new alarm configuration file: %s", path)
		} else {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	}

	return s, nil
}

// loadConfig loads the alarm configuration from file.
//
// On success it replaces s.config and records lastLoadTime. It returns the
// os.ReadFile error unwrapped (so callers can test os.IsNotExist) and a
// wrapped error if the JSON cannot be parsed.
func (s *Server) loadConfig() error {
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return err
	}

	var config alarm.AlarmConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	s.config = &config
	s.lastLoadTime = time.Now()
	return nil
}

// saveConfig saves the alarm configuration to file.
//
// It writes s.config as indented JSON to s.configPath with mode 0644,
// overwriting the file in place (not atomically). It returns a wrapped error
// if marshalling or writing fails.
func (s *Server) saveConfig() error {
	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	logger.Info("Saved alarm configuration to: %s", s.configPath)
	return nil
}

// loadContacts loads the contact list from the CONTACT_LIST environment variable or from the env file.
//
// When s.envFile is set, the file is scanned line by line for
// CONTACT_LIST='...' and the single-quoted value may span multiple lines;
// otherwise the CONTACT_LIST environment variable is used. An empty or
// missing value yields an empty list. Entries with an empty name, no email
// and no SMS, an email without "@", or an SMS without a leading "+" are only
// logged as warnings, not rejected. It returns an error if the env file
// cannot be read or the value is not a valid JSON array of Contact.
func (s *Server) loadContacts() error {
	var contactListJSON string

	if s.envFile != "" {
		// Read from the specified env file
		content, err := os.ReadFile(s.envFile)
		if err != nil {
			return fmt.Errorf("failed to read env file '%s': %w", s.envFile, err)
		}

		contentStr := string(content)
		lines := strings.Split(contentStr, "\n")
		inContactList := false
		for _, line := range lines {
			if strings.HasPrefix(line, "CONTACT_LIST=") {
				// Find the opening quote
				quoteIndex := strings.Index(line, "'")
				if quoteIndex >= 0 {
					contactListJSON += line[quoteIndex+1:]
					if strings.HasSuffix(line, "'") {
						// Single line
						contactListJSON = strings.TrimSuffix(contactListJSON, "'")
						break
					} else {
						inContactList = true
					}
				}
			} else if inContactList {
				contactListJSON += line + "\n"
				if strings.Contains(line, "'") {
					// Found closing quote
					quoteIndex := strings.Index(line, "'")
					contactListJSON = contactListJSON[:len(contactListJSON)-len(line)-1+quoteIndex]
					break
				}
			}
		}
	} else {
		// Read from environment variable
		contactListJSON = os.Getenv("CONTACT_LIST")
	}

	if contactListJSON == "" {
		// No contact list configured, use empty list
		s.contacts = []Contact{}
		return nil
	}

	var contacts []Contact
	if err := json.Unmarshal([]byte(contactListJSON), &contacts); err != nil {
		return fmt.Errorf("failed to parse CONTACT_LIST JSON: %w", err)
	}

	// Validate contact structure
	for i, contact := range contacts {
		if contact.Name == "" {
			logger.Warn("CONTACT_LIST: Contact %d has empty name field", i+1)
		}
		if contact.Email == "" && contact.SMS == "" {
			logger.Warn("CONTACT_LIST: Contact %d (%s) has neither email nor SMS configured", i+1, contact.Name)
		}
		if contact.Email != "" && !strings.Contains(contact.Email, "@") {
			logger.Warn("CONTACT_LIST: Contact %d (%s) has invalid email format: %s", i+1, contact.Name, contact.Email)
		}
		if contact.SMS != "" && !strings.HasPrefix(contact.SMS, "+") {
			logger.Warn("CONTACT_LIST: Contact %d (%s) SMS number should start with '+': %s", i+1, contact.Name, contact.SMS)
		}
	}

	s.contacts = contacts
	logger.Info("Loaded %d contacts from CONTACT_LIST", len(contacts))
	return nil
}

// Start starts the alarm editor web server.
//
// It registers the editor page, the static-file handler and all /api/
// routes on a new ServeMux and blocks in http.ListenAndServe on ":"+port.
// It returns only when the listener fails.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Main editor page
	mux.HandleFunc("/", s.handleIndex)

	// Static files
	mux.HandleFunc("/alarm-editor/static/", s.handleStaticFiles)

	// API endpoints
	mux.HandleFunc("/api/config", s.handleGetConfig)
	mux.HandleFunc("/api/config/save", s.handleSaveConfig)
	mux.HandleFunc("/api/alarms", s.handleListAlarms)
	mux.HandleFunc("/api/alarms/create", s.handleCreateAlarm)
	mux.HandleFunc("/api/alarms/update", s.handleUpdateAlarm)
	mux.HandleFunc("/api/alarms/delete", s.handleDeleteAlarm)
	mux.HandleFunc("/api/tags", s.handleGetTags)
	mux.HandleFunc("/api/tags/save", s.handleSaveTags)
	mux.HandleFunc("/api/validate", s.handleValidate)
	mux.HandleFunc("/api/validate-json", s.handleValidateJSON)
	mux.HandleFunc("/api/fields", s.handleGetFields)
	mux.HandleFunc("/api/env-defaults", s.handleGetEnvDefaults)
	mux.HandleFunc("/api/contacts", s.handleGetContacts)
	mux.HandleFunc("/api/contacts/save", s.handleSaveContacts)

	addr := ":" + s.port
	logger.Info("Starting Alarm Editor on http://localhost%s", addr)
	logger.Info("Editing: %s", s.configPath)
	logger.Info("Press Ctrl+C to stop")

	return http.ListenAndServe(addr, mux)
}

// handleStaticFiles serves static CSS and JS files.
//
// Files under /alarm-editor/static/ are served from
// ./pkg/alarm/editor/static/ relative to the process working directory, so
// the binary must be run from the repository root for them to be found.
// CSS and JS responses are marked no-cache so edits show up without a hard
// reload.
func (s *Server) handleStaticFiles(w http.ResponseWriter, r *http.Request) {
	// Extract filename from URL path
	filename := strings.TrimPrefix(r.URL.Path, "/alarm-editor/static/")

	logger.Debug("Static file request: %s (path: %s)", filename, r.URL.Path)

	// Serve the file from the physical directory
	filePath := "./pkg/alarm/editor/static/" + filename

	// Set appropriate content type
	switch {
	case strings.HasSuffix(filename, ".css"):
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	case strings.HasSuffix(filename, ".js"):
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	}

	http.ServeFile(w, r, filePath)
}

// handleIndex serves the main editor HTML page.
//
// It renders indexHTML as an html/template with ConfigPath, Port, Version,
// EnvFile and LastLoad, responding 500 if the template fails to parse or
// execute.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("index").Parse(indexHTML)
	if err != nil {
		logger.Error("Failed to parse HTML template: %v", err)
		http.Error(w, "Internal Server Error: Failed to parse template", http.StatusInternalServerError)
		return
	}

	lastLoad := "never"
	if !s.lastLoadTime.IsZero() {
		lastLoad = s.lastLoadTime.Format("2006-01-02 15:04:05")
	}
	data := map[string]interface{}{
		"ConfigPath": s.configPath,
		"Port":       s.port,
		"Version":    s.version,
		"EnvFile":    s.envFile,
		"LastLoad":   lastLoad,
	}
	if err := tmpl.Execute(w, data); err != nil {
		logger.Error("Failed to execute template: %v", err)
		http.Error(w, "Internal Server Error: Failed to render page", http.StatusInternalServerError)
		return
	}
}

// handleGetConfig returns the full alarm configuration as JSON (any method).
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.config)
}

// handleSaveConfig saves the entire configuration.
//
// It accepts POST only (405 otherwise), decodes an alarm.AlarmConfig from the
// body (400 on bad JSON), runs its Validate method (400 on failure), then
// replaces the in-memory config and writes it to disk (500 on write failure).
// On success it responds {"status":"success"}.
func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var config alarm.AlarmConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate the configuration
	if err := config.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Invalid configuration: %v", err), http.StatusBadRequest)
		return
	}

	s.config = &config
	if err := s.saveConfig(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleListAlarms returns all alarms with optional filtering.
//
// Query parameter "name" keeps alarms whose name contains it
// (case-insensitive); "tag" keeps alarms carrying exactly that tag. Both
// filters may be combined. The response is always a JSON array, never null.
func (s *Server) handleListAlarms(w http.ResponseWriter, r *http.Request) {
	nameFilter := r.URL.Query().Get("name")
	tagFilter := r.URL.Query().Get("tag")

	alarms := s.config.Alarms
	filtered := []alarm.Alarm{}

	for _, a := range alarms {
		// Apply name filter (case-insensitive substring match)
		if nameFilter != "" && !strings.Contains(strings.ToLower(a.Name), strings.ToLower(nameFilter)) {
			continue
		}

		// Apply tag filter
		if tagFilter != "" {
			hasTag := false
			for _, tag := range a.Tags {
				if tag == tagFilter {
					hasTag = true
					break
				}
			}
			if !hasTag {
				continue
			}
		}

		filtered = append(filtered, a)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(filtered)
}

// handleCreateAlarm creates a new alarm.
//
// It accepts POST only (405 otherwise) and decodes an alarm.Alarm from the
// body (400 on bad JSON). It responds 409 if an alarm with the same name
// exists and 400 if any channel fails Validate; otherwise the alarm is
// appended and the config saved (500 on write failure). Unlike
// handleSaveConfig it does not run full config validation.
func (s *Server) handleCreateAlarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var newAlarm alarm.Alarm
	if err := json.NewDecoder(r.Body).Decode(&newAlarm); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Check for duplicate name
	for _, a := range s.config.Alarms {
		if a.Name == newAlarm.Name {
			http.Error(w, fmt.Sprintf("Alarm with name '%s' already exists", newAlarm.Name), http.StatusConflict)
			return
		}
	}

	// Validate channels
	for i, ch := range newAlarm.Channels {
		if err := ch.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("Channel %d validation failed: %v", i, err), http.StatusBadRequest)
			return
		}
	}

	s.config.Alarms = append(s.config.Alarms, newAlarm)

	if err := s.saveConfig(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleUpdateAlarm updates an existing alarm.
//
// It accepts POST only (405 otherwise) and decodes the replacement
// alarm.Alarm from the body (400 on bad JSON). The alarm to replace is found
// by the "oldName" query parameter, defaulting to the body's name, which is
// how renames are expressed. It responds 409 if a rename collides with
// another alarm, 400 if a channel fails Validate, 404 if oldName is not
// found, and 500 if saving fails.
func (s *Server) handleUpdateAlarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var updatedAlarm alarm.Alarm
	if err := json.NewDecoder(r.Body).Decode(&updatedAlarm); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Get original name from query parameter (for name changes)
	oldName := r.URL.Query().Get("oldName")
	if oldName == "" {
		oldName = updatedAlarm.Name // Default to current name if not changing
	}

	// Check for duplicate name (if name is changing)
	if oldName != updatedAlarm.Name {
		for _, a := range s.config.Alarms {
			if a.Name == updatedAlarm.Name {
				http.Error(w, fmt.Sprintf("Alarm with name '%s' already exists", updatedAlarm.Name), http.StatusConflict)
				return
			}
		}
	}

	// Find and update the alarm by old name
	found := false
	for i, a := range s.config.Alarms {
		if a.Name == oldName {
			// Validate channels
			for j, ch := range updatedAlarm.Channels {
				if err := ch.Validate(); err != nil {
					http.Error(w, fmt.Sprintf("Channel %d validation failed: %v", j, err), http.StatusBadRequest)
					return
				}
			}

			s.config.Alarms[i] = updatedAlarm
			found = true
			break
		}
	}

	if !found {
		http.Error(w, fmt.Sprintf("Alarm '%s' not found", oldName), http.StatusNotFound)
		return
	}

	if err := s.saveConfig(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleDeleteAlarm deletes an alarm by name.
//
// It accepts POST only (405 otherwise) and takes the name from the "name"
// query parameter (400 if missing). It responds 404 if no alarm has that
// name and 500 if saving fails. All alarms with a matching name are removed.
func (s *Server) handleDeleteAlarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Name parameter required", http.StatusBadRequest)
		return
	}

	// Find and remove the alarm
	newAlarms := []alarm.Alarm{}
	found := false
	for _, a := range s.config.Alarms {
		if a.Name == name {
			found = true
			continue
		}
		newAlarms = append(newAlarms, a)
	}

	if !found {
		http.Error(w, fmt.Sprintf("Alarm '%s' not found", name), http.StatusNotFound)
		return
	}

	s.config.Alarms = newAlarms

	if err := s.saveConfig(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleGetTags returns all unique tags from all alarms.
//
// The result also includes predefined tags from TAG_LIST, read from s.envFile
// (single-quoted, possibly multi-line, as in loadContacts) or from the
// environment. Read or parse problems with TAG_LIST, and suspicious tags
// (with spaces or longer than 50 characters), are only logged. Empty tags
// are skipped. The response is a sorted JSON array of strings.
func (s *Server) handleGetTags(w http.ResponseWriter, r *http.Request) {
	tagSet := make(map[string]bool)

	// Add tags from existing alarms
	for _, a := range s.config.Alarms {
		for _, tag := range a.Tags {
			tagSet[tag] = true
		}
	}

	// Add predefined tags from environment
	var predefinedTagsJSON string
	if s.envFile != "" {
		// Read from the specified env file
		content, err := os.ReadFile(s.envFile)
		if err != nil {
			logger.Warn("Failed to read env file '%s': %v", s.envFile, err)
		} else {
			contentStr := string(content)
			lines := strings.Split(contentStr, "\n")
			inTagList := false
			for _, line := range lines {
				if strings.HasPrefix(line, "TAG_LIST=") {
					// Find the opening quote
					quoteIndex := strings.Index(line, "'")
					if quoteIndex >= 0 {
						predefinedTagsJSON += line[quoteIndex+1:]
						if strings.HasSuffix(line, "'") {
							// Single line
							predefinedTagsJSON = strings.TrimSuffix(predefinedTagsJSON, "'")
							break
						} else {
							inTagList = true
						}
					}
				} else if inTagList {
					predefinedTagsJSON += line + "\n"
					if strings.Contains(line, "'") {
						// Found closing quote
						quoteIndex := strings.Index(line, "'")
						predefinedTagsJSON = predefinedTagsJSON[:len(predefinedTagsJSON)-len(line)-1+quoteIndex]
						break
					}
				}
			}
		}
	} else {
		// Read from environment variable
		predefinedTagsJSON = os.Getenv("TAG_LIST")
	}

	if predefinedTagsJSON != "" {
		var predefinedTags []string
		if err := json.Unmarshal([]byte(predefinedTagsJSON), &predefinedTags); err != nil {
			logger.Warn("Failed to parse TAG_LIST JSON: %v", err)
		} else {
			// Validate tag format
			for i, tag := range predefinedTags {
				tag = strings.TrimSpace(tag)
				if tag == "" {
					logger.Warn("TAG_LIST: Tag %d is empty or whitespace-only", i+1)
					continue
				}
				if strings.Contains(tag, " ") {
					logger.Warn("TAG_LIST: Tag %d contains spaces (use hyphens or underscores): %s", i+1, tag)
				}
				if len(tag) > 50 {
					logger.Warn("TAG_LIST: Tag %d is very long (%d chars), consider shortening: %s", i+1, len(tag), tag)
				}
				tagSet[tag] = true
			}
		}
	}

	tags := []string{}
	for tag := range tagSet {
		tags = append(tags, tag)
	}

	// Sort tags alphabetically
	sort.Strings(tags)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tags)
}

// handleValidate validates an alarm condition.
//
// It accepts POST only (405 otherwise) with a body {"condition": "..."}
// (400 on bad JSON). It first applies syntactic checks: empty input, a
// leading or trailing && or ||, and empty operands. Only one operator kind is
// split on (&& takes precedence), so a mixed expression's || operands are not
// checked here. The condition is then evaluated against a fixed sample
// observation with alarm.Evaluator.EvaluateWithAlarm. The response is
// {"valid": bool} plus "error" when invalid or "paraphrase" (a plain-English
// rendering) when valid; validation failures are reported with status 200.
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Condition string `json:"condition"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create a test observation with reasonable values
	testObs := &weather.Observation{
		AirTemperature:   20.0,
		RelativeHumidity: 50.0,
		StationPressure:  1013.25,
		WindAvg:          5.0,
		Illuminance:      10000,
		UV:               3,
	}

	// Validate condition format before evaluation
	condition := strings.TrimSpace(req.Condition)

	var validationErr error

	if condition == "" {
		validationErr = fmt.Errorf("condition cannot be empty")
	} else if strings.HasSuffix(condition, "&&") || strings.HasSuffix(condition, "||") {
		validationErr = fmt.Errorf("condition ends with incomplete operator (&&/||)")
	} else if strings.HasPrefix(condition, "&&") || strings.HasPrefix(condition, "||") {
		validationErr = fmt.Errorf("condition starts with incomplete operator (&&/||)")
	} else if strings.Contains(condition, "&&") || strings.Contains(condition, "||") {
		// Check for empty parts in compound conditions
		// Split on the actual operators, not individual characters
		var parts []string
		if strings.Contains(condition, "&&") {
			parts = strings.Split(condition, "&&")
		} else {
			parts = strings.Split(condition, "||")
		}

		for i, part := range parts {
			if strings.TrimSpace(part) == "" {
				validationErr = fmt.Errorf("logical operator (&&/||) requires expressions on both sides (missing expression at position %d)", i+1)
				break
			}
		}
	}

	response := map[string]interface{}{}

	if validationErr != nil {
		response["valid"] = false
		response["error"] = validationErr.Error()
	} else {
		// Create a dummy alarm context for change detection validation
		dummyAlarm := &alarm.Alarm{
			Name: "validation-test",
		}

		evaluator := alarm.NewEvaluator()
		_, err := evaluator.EvaluateWithAlarm(condition, testObs, dummyAlarm)

		response["valid"] = err == nil

		if err != nil {
			response["error"] = err.Error()
		} else {
			// Add paraphrase for valid conditions
			response["paraphrase"] = evaluator.Paraphrase(condition)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// handleValidateJSON validates a JSON message template.
//
// It accepts POST only (405 otherwise) with a body {"template": "..."} (400
// on bad JSON). Known {{placeholder}} tokens, including the composite
// {{alarm_info}} and {{sensor_info}} objects, are replaced with fixed sample
// values by simple string substitution (this mirrors, but does not call, the
// real notifier's expansion), and the result is parsed as JSON. The response
// is {"valid", "expanded", and "error" or "message"} with status 200 either
// way.
func (s *Server) handleValidateJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Template string `json:"template"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create test data for template expansion
	testObs := &weather.Observation{
		Timestamp:            time.Now().Unix(),
		AirTemperature:       25.5,
		RelativeHumidity:     65.0,
		StationPressure:      1013.25,
		WindAvg:              8.5,
		WindGust:             12.3,
		WindDirection:        180.0,
		Illuminance:          50000,
		UV:                   6,
		RainAccumulated:      2.5,
		RainDailyTotal:       15.2,
		LightningStrikeCount: 3,
		// LightningStrikeAvg is not used in template validation
	}

	testAlarm := &alarm.Alarm{
		Name:           "test-alarm",
		Description:    "Test alarm for validation",
		Condition:      "temperature > 20",
		Enabled:        true,
		TriggeredCount: 5,
	}

	// Simple template expansion for validation
	expanded := strings.ReplaceAll(req.Template, "{{timestamp}}", time.Unix(testObs.Timestamp, 0).Format("2006-01-02 15:04:05"))
	expanded = strings.ReplaceAll(expanded, "{{alarm_name}}", testAlarm.Name)
	expanded = strings.ReplaceAll(expanded, "{{alarm_description}}", testAlarm.Description)
	expanded = strings.ReplaceAll(expanded, "{{alarm_condition}}", testAlarm.Condition)
	expanded = strings.ReplaceAll(expanded, "{{station}}", "Test Station")
	expanded = strings.ReplaceAll(expanded, "{{temperature}}", fmt.Sprintf("%.1f", testObs.AirTemperature))
	expanded = strings.ReplaceAll(expanded, "{{humidity}}", fmt.Sprintf("%.0f", testObs.RelativeHumidity))
	expanded = strings.ReplaceAll(expanded, "{{pressure}}", fmt.Sprintf("%.2f", testObs.StationPressure))
	expanded = strings.ReplaceAll(expanded, "{{wind_speed}}", fmt.Sprintf("%.1f", testObs.WindAvg))
	expanded = strings.ReplaceAll(expanded, "{{wind_gust}}", fmt.Sprintf("%.1f", testObs.WindGust))
	expanded = strings.ReplaceAll(expanded, "{{wind_direction}}", fmt.Sprintf("%.0f", testObs.WindDirection))
	expanded = strings.ReplaceAll(expanded, "{{lux}}", fmt.Sprintf("%.0f", testObs.Illuminance))
	expanded = strings.ReplaceAll(expanded, "{{uv}}", fmt.Sprintf("%d", testObs.UV))
	expanded = strings.ReplaceAll(expanded, "{{rain_rate}}", fmt.Sprintf("%.2f", testObs.RainAccumulated))
	expanded = strings.ReplaceAll(expanded, "{{rain_daily}}", fmt.Sprintf("%.2f", testObs.RainDailyTotal))
	expanded = strings.ReplaceAll(expanded, "{{lightning_count}}", fmt.Sprintf("%d", testObs.LightningStrikeCount))

	// Handle complex templates that need JSON formatting
	expanded = strings.ReplaceAll(expanded, "{{alarm_info}}", fmt.Sprintf(`{"name":"%s","description":"%s","condition":"%s","enabled":%t,"triggered_count":%d}`,
		testAlarm.Name, testAlarm.Description, testAlarm.Condition, testAlarm.Enabled, testAlarm.TriggeredCount))
	expanded = strings.ReplaceAll(expanded, "{{sensor_info}}", fmt.Sprintf(`{"temperature":%.1f,"humidity":%.0f,"pressure":%.2f,"wind_speed":%.1f,"wind_gust":%.1f,"wind_direction":%.0f,"lux":%.0f,"uv":%d,"rain_rate":%.2f,"rain_daily":%.2f,"lightning_count":%d}`,
		testObs.AirTemperature, testObs.RelativeHumidity, testObs.StationPressure, testObs.WindAvg, testObs.WindGust, testObs.WindDirection, testObs.Illuminance, testObs.UV, testObs.RainAccumulated, testObs.RainDailyTotal, testObs.LightningStrikeCount))

	// Try to parse the expanded result as JSON
	var jsonTest interface{}
	err := json.Unmarshal([]byte(expanded), &jsonTest)

	response := map[string]interface{}{}

	if err != nil {
		response["valid"] = false
		response["error"] = fmt.Sprintf("Template expansion produces invalid JSON: %v", err)
		response["expanded"] = expanded
	} else {
		response["valid"] = true
		response["message"] = "Template produces valid JSON"
		response["expanded"] = expanded
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// handleGetFields returns the field names available in alarm conditions, as
// reported by alarm.Evaluator.GetAvailableFields.
func (s *Server) handleGetFields(w http.ResponseWriter, r *http.Request) {
	evaluator := alarm.NewEvaluator()
	fields := evaluator.GetAvailableFields()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fields)
}

// handleGetEnvDefaults returns default values from environment variables.
//
// The response is {"emailTo": $MS365_TO_ADDRESS, "smsTo": $SMS_TO_NUMBER},
// used to prefill new notification channels. Unlike contacts and tags, these
// are always read from the process environment, never from s.envFile.
func (s *Server) handleGetEnvDefaults(w http.ResponseWriter, r *http.Request) {
	defaults := map[string]string{
		"emailTo": os.Getenv("MS365_TO_ADDRESS"),
		"smsTo":   os.Getenv("SMS_TO_NUMBER"),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(defaults)
}

// handleGetContacts returns the contact list for dropdowns, sorted by name.
//
// It sorts a copy so the stored order in s.contacts is unchanged.
func (s *Server) handleGetContacts(w http.ResponseWriter, r *http.Request) {
	// Create a copy of contacts to sort
	sortedContacts := make([]Contact, len(s.contacts))
	copy(sortedContacts, s.contacts)

	// Sort contacts alphabetically by name
	sort.Slice(sortedContacts, func(i, j int) bool {
		return sortedContacts[i].Name < sortedContacts[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sortedContacts)
}

// handleSaveContacts saves the contact list.
//
// It accepts POST only (405 otherwise) with a body {"contacts": [...],
// "saveType": "json"|"env"}. Each contact must have a name and an email or
// SMS, emails must contain "@" and SMS numbers must start with "+" (400
// otherwise). The in-memory list is replaced before the save type is checked.
// "json" writes contacts.json in the working directory; "env" rewrites the
// CONTACT_LIST='...' block (single- or multi-line) in s.envFile, else ".env",
// falling back to ".env.example" if that does not exist, appending the
// variable if absent. Any other saveType is a 400. Responds {"message": ...}.
func (s *Server) handleSaveContacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Contacts []Contact `json:"contacts"`
		SaveType string    `json:"saveType"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate contacts
	for i, contact := range req.Contacts {
		if contact.Name == "" {
			http.Error(w, fmt.Sprintf("Contact %d has empty name", i+1), http.StatusBadRequest)
			return
		}
		if contact.Email == "" && contact.SMS == "" {
			http.Error(w, fmt.Sprintf("Contact %d (%s) must have either email or SMS", i+1, contact.Name), http.StatusBadRequest)
			return
		}
		if contact.Email != "" && !strings.Contains(contact.Email, "@") {
			http.Error(w, fmt.Sprintf("Contact %d (%s) has invalid email format", i+1, contact.Name), http.StatusBadRequest)
			return
		}
		if contact.SMS != "" && !strings.HasPrefix(contact.SMS, "+") {
			http.Error(w, fmt.Sprintf("Contact %d (%s) SMS must start with '+'", i+1, contact.Name), http.StatusBadRequest)
			return
		}
	}

	// Update server contacts
	s.contacts = req.Contacts

	// Save based on type
	var message string
	if req.SaveType == "json" {
		// Save as JSON file
		contactsJSON, err := json.MarshalIndent(req.Contacts, "", "  ")
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to marshal contacts: %v", err), http.StatusInternalServerError)
			return
		}

		filename := "contacts.json"
		if err := os.WriteFile(filename, contactsJSON, 0644); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save contacts file: %v", err), http.StatusInternalServerError)
			return
		}
		message = fmt.Sprintf("Contacts saved to %s", filename)
	} else if req.SaveType == "env" {
		// Update .env file
		envFile := s.envFile
		if envFile == "" {
			envFile = ".env"
		}
		if _, err := os.Stat(envFile); os.IsNotExist(err) {
			envFile = ".env.example"
		}

		content, err := os.ReadFile(envFile)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to read env file: %v", err), http.StatusInternalServerError)
			return
		}

		contactsJSON, err := json.MarshalIndent(req.Contacts, "", "  ")
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to marshal contacts: %v", err), http.StatusInternalServerError)
			return
		}

		// Replace CONTACT_LIST in .env file, handling multi-line values
		contentStr := string(content)
		lines := strings.Split(contentStr, "\n")
		startLine := -1
		endLine := -1
		for i, line := range lines {
			if strings.HasPrefix(line, "CONTACT_LIST=") {
				startLine = i
				// Only an opening quote that is not closed on this line starts a
				// multi-line value; an unquoted value is a single line.
				value := strings.TrimPrefix(line, "CONTACT_LIST=")
				if !strings.HasPrefix(value, "'") || (len(value) > 1 && strings.HasSuffix(value, "'")) {
					endLine = i
					break
				}
			} else if startLine != -1 {
				if strings.Contains(line, "'") {
					endLine = i
					break
				}
			}
		}
		if startLine != -1 && endLine == -1 {
			// Unterminated quoted value: replace just the key's line rather than
			// letting lines[endLine+1:] re-append the whole file.
			endLine = startLine
		}

		newLines := strings.Split(fmt.Sprintf("CONTACT_LIST='%s'", string(contactsJSON)), "\n")

		if startLine != -1 {
			// Replace the block
			lines = append(lines[:startLine], append(newLines, lines[endLine+1:]...)...)
		} else {
			// Append
			lines = append(lines, newLines...)
		}

		contentStr = strings.Join(lines, "\n")

		if err := os.WriteFile(envFile, []byte(contentStr), 0644); err != nil {
			http.Error(w, fmt.Sprintf("Failed to update env file: %v", err), http.StatusInternalServerError)
			return
		}
		message = fmt.Sprintf("Contacts updated in %s", envFile)
	} else {
		http.Error(w, "Invalid save type", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
}

// handleSaveTags saves the tag list.
//
// It accepts POST only (405 otherwise) with a body {"tags": [...],
// "saveType": "json"|"env"}. Tags are trimmed and must be non-empty, contain
// no spaces and be at most 50 characters (400 otherwise). "json" writes
// tags.json in the working directory; "env" rewrites the TAG_LIST='...' block
// using the same file selection and replacement rules as handleSaveContacts.
// Any other saveType is a 400. The in-memory config is not changed, since
// tags shown by handleGetTags are re-read from the source each time.
// Responds {"message": ...}.
func (s *Server) handleSaveTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Tags     []string `json:"tags"`
		SaveType string   `json:"saveType"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate tags
	for i, tag := range req.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			http.Error(w, fmt.Sprintf("Tag %d is empty", i+1), http.StatusBadRequest)
			return
		}
		if strings.Contains(tag, " ") {
			http.Error(w, fmt.Sprintf("Tag %d contains spaces (use hyphens or underscores): %s", i+1, tag), http.StatusBadRequest)
			return
		}
		if len(tag) > 50 {
			http.Error(w, fmt.Sprintf("Tag %d is too long (%d chars, max 50): %s", i+1, len(tag), tag), http.StatusBadRequest)
			return
		}
		req.Tags[i] = tag // Update with trimmed version
	}

	// Save based on type
	var message string
	if req.SaveType == "json" {
		// Save as JSON file
		tagsJSON, err := json.MarshalIndent(req.Tags, "", "  ")
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to marshal tags: %v", err), http.StatusInternalServerError)
			return
		}

		filename := "tags.json"
		if err := os.WriteFile(filename, tagsJSON, 0644); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save tags file: %v", err), http.StatusInternalServerError)
			return
		}
		message = fmt.Sprintf("Tags saved to %s", filename)
	} else if req.SaveType == "env" {
		// Update .env file
		envFile := s.envFile
		if envFile == "" {
			envFile = ".env"
		}
		if _, err := os.Stat(envFile); os.IsNotExist(err) {
			envFile = ".env.example"
		}

		content, err := os.ReadFile(envFile)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to read env file: %v", err), http.StatusInternalServerError)
			return
		}

		tagsJSON, err := json.MarshalIndent(req.Tags, "", "  ")
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to marshal tags: %v", err), http.StatusInternalServerError)
			return
		}

		// Replace TAG_LIST in .env file, handling multi-line values
		contentStr := string(content)
		lines := strings.Split(contentStr, "\n")
		startLine := -1
		endLine := -1
		for i, line := range lines {
			if strings.HasPrefix(line, "TAG_LIST=") {
				startLine = i
				// Only an opening quote that is not closed on this line starts a
				// multi-line value; an unquoted value is a single line.
				value := strings.TrimPrefix(line, "TAG_LIST=")
				if !strings.HasPrefix(value, "'") || (len(value) > 1 && strings.HasSuffix(value, "'")) {
					endLine = i
					break
				}
			} else if startLine != -1 {
				if strings.Contains(line, "'") {
					endLine = i
					break
				}
			}
		}
		if startLine != -1 && endLine == -1 {
			// Unterminated quoted value: replace just the key's line rather than
			// letting lines[endLine+1:] re-append the whole file.
			endLine = startLine
		}

		newLines := strings.Split(fmt.Sprintf("TAG_LIST='%s'", string(tagsJSON)), "\n")

		if startLine != -1 {
			// Replace the block
			lines = append(lines[:startLine], append(newLines, lines[endLine+1:]...)...)
		} else {
			// Append
			lines = append(lines, newLines...)
		}

		contentStr = strings.Join(lines, "\n")

		if err := os.WriteFile(envFile, []byte(contentStr), 0644); err != nil {
			http.Error(w, fmt.Sprintf("Failed to update env file: %v", err), http.StatusInternalServerError)
			return
		}
		message = fmt.Sprintf("Tags updated in %s", envFile)
	} else {
		http.Error(w, "Invalid save type", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
}
