package editor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"tempest-homekit-go/pkg/alarm"
	"testing"
)

// TestNewServer checks that NewServer strips the "@" prefix from the config
// path, loads the existing config file and stores the port it was given.
func TestNewServer(t *testing.T) {
	// Create a temporary config file
	tmpfile, err := os.CreateTemp("", "alarms_test_*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	// Write a minimal config
	config := alarm.AlarmConfig{
		Alarms: []alarm.Alarm{
			{
				Name:      "test-alarm",
				Condition: "temperature > 85",
				Enabled:   true,
			},
		},
	}
	data, _ := json.Marshal(config)
	if _, err := tmpfile.Write(data); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	// Create server
	server, err := NewServer("@"+tmpfile.Name(), "8081", "test", ".env")
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	if server.configPath != tmpfile.Name() {
		t.Errorf("Expected configPath %s, got %s", tmpfile.Name(), server.configPath)
	}

	if server.port != "8081" {
		t.Errorf("Expected port 8081, got %s", server.port)
	}

	if len(server.config.Alarms) != 1 {
		t.Errorf("Expected 1 alarm, got %d", len(server.config.Alarms))
	}
}

// TestHandleGetConfig checks that handleGetConfig returns the in-memory
// configuration as JSON.
func TestHandleGetConfig(t *testing.T) {
	// Create test server
	config := &alarm.AlarmConfig{
		Alarms: []alarm.Alarm{
			{
				Name:      "test-alarm",
				Condition: "temperature > 85",
				Enabled:   true,
			},
		},
	}

	server := &Server{
		configPath: "test.json",
		port:       "8081",
		config:     config,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()

	server.handleGetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result alarm.AlarmConfig
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(result.Alarms) != 1 {
		t.Errorf("Expected 1 alarm, got %d", len(result.Alarms))
	}

	if result.Alarms[0].Name != "test-alarm" {
		t.Errorf("Expected alarm name 'test-alarm', got '%s'", result.Alarms[0].Name)
	}
}

// TestHandleListAlarms checks handleListAlarms with no filter, a name filter
// and a tag filter.
func TestHandleListAlarms(t *testing.T) {
	config := &alarm.AlarmConfig{
		Alarms: []alarm.Alarm{
			{
				Name:      "high-temp",
				Condition: "temperature > 85",
				Tags:      []string{"temperature", "heat"},
				Enabled:   true,
			},
			{
				Name:      "lightning",
				Condition: "lightning_strike_count > 0",
				Tags:      []string{"lightning"},
				Enabled:   true,
			},
		},
	}

	server := &Server{
		configPath: "test.json",
		port:       "8081",
		config:     config,
	}

	// Test without filter
	req := httptest.NewRequest(http.MethodGet, "/api/alarms", nil)
	w := httptest.NewRecorder()
	server.handleListAlarms(w, req)

	var result []alarm.Alarm
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("Expected 2 alarms, got %d", len(result))
	}

	// Test with name filter
	req = httptest.NewRequest(http.MethodGet, "/api/alarms?name=temp", nil)
	w = httptest.NewRecorder()
	server.handleListAlarms(w, req)

	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("Expected 1 alarm with name filter, got %d", len(result))
	}
	if result[0].Name != "high-temp" {
		t.Errorf("Expected 'high-temp', got '%s'", result[0].Name)
	}

	// Test with tag filter
	req = httptest.NewRequest(http.MethodGet, "/api/alarms?tag=lightning", nil)
	w = httptest.NewRecorder()
	server.handleListAlarms(w, req)

	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("Expected 1 alarm with tag filter, got %d", len(result))
	}
	if result[0].Name != "lightning" {
		t.Errorf("Expected 'lightning', got '%s'", result[0].Name)
	}
}

// TestHandleGetTags checks that handleGetTags returns the de-duplicated,
// sorted union of the alarms' tags.
func TestHandleGetTags(t *testing.T) {
	config := &alarm.AlarmConfig{
		Alarms: []alarm.Alarm{
			{
				Name: "alarm1",
				Tags: []string{"temperature", "heat"},
			},
			{
				Name: "alarm2",
				Tags: []string{"temperature", "cold"},
			},
			{
				Name: "alarm3",
				Tags: []string{"lightning"},
			},
		},
	}

	server := &Server{
		configPath: "test.json",
		port:       "8081",
		config:     config,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	w := httptest.NewRecorder()
	server.handleGetTags(w, req)

	var tags []string
	if err := json.NewDecoder(w.Body).Decode(&tags); err != nil {
		t.Fatalf("failed to decode tags response: %v", err)
	}

	// Check that we get unique tags sorted alphabetically
	expectedTags := map[string]bool{
		"temperature": false,
		"heat":        false,
		"cold":        false,
		"lightning":   false,
	}

	for _, tag := range tags {
		if _, exists := expectedTags[tag]; exists {
			expectedTags[tag] = true
		}
	}

	for tag, found := range expectedTags {
		if !found {
			t.Errorf("Expected tag '%s' not found", tag)
		}
	}

	// Check that tags are sorted alphabetically
	if len(tags) > 1 {
		for i := 1; i < len(tags); i++ {
			if tags[i-1] > tags[i] {
				t.Errorf("Tags not sorted alphabetically: %v", tags)
				break
			}
		}
	}
}

// TestHandleGetTags_WithPredefinedTags checks that tags from the TAG_LIST
// environment variable are merged with alarm tags.
func TestHandleGetTags_WithPredefinedTags(t *testing.T) {
	// Set up environment with predefined tags
	originalTagList := os.Getenv("TAG_LIST")

	_ = os.Setenv("TAG_LIST", `["weather", "alert", "storm", "temperature"]`)
	defer func() { _ = os.Setenv("TAG_LIST", originalTagList) }()

	config := &alarm.AlarmConfig{
		Alarms: []alarm.Alarm{
			{
				Name: "alarm1",
				Tags: []string{"temperature", "heat"},
			},
			{
				Name: "alarm2",
				Tags: []string{"lightning"},
			},
		},
	}

	server := &Server{
		configPath: "test.json",
		port:       "8081",
		config:     config,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	w := httptest.NewRecorder()
	server.handleGetTags(w, req)

	var tags []string
	if err := json.NewDecoder(w.Body).Decode(&tags); err != nil {
		t.Fatalf("failed to decode tags response: %v", err)
	}

	// Check that we get both alarm tags and predefined tags
	expectedTags := map[string]bool{
		"temperature": false, // from alarm
		"heat":        false, // from alarm
		"lightning":   false, // from alarm
		"weather":     false, // from predefined
		"alert":       false, // from predefined
		"storm":       false, // from predefined
	}

	for _, tag := range tags {
		if _, exists := expectedTags[tag]; exists {
			expectedTags[tag] = true
		}
	}

	for tag, found := range expectedTags {
		if !found {
			t.Errorf("Expected tag '%s' not found", tag)
		}
	}

	// Check that tags are sorted alphabetically
	if len(tags) > 1 {
		for i := 1; i < len(tags); i++ {
			if tags[i-1] > tags[i] {
				t.Errorf("Tags not sorted alphabetically: %v", tags)
				break
			}
		}
	}
}

// TestHandleValidate table-tests handleValidate's valid/invalid verdict for
// well-formed and malformed conditions.
func TestHandleValidate(t *testing.T) {
	server := &Server{
		configPath: "test.json",
		port:       "8081",
		config:     &alarm.AlarmConfig{},
	}

	tests := []struct {
		name      string
		condition string
		wantValid bool
	}{
		{"valid simple", "temperature > 85", true},
		{"valid compound", "temperature > 85 && humidity > 80", true},
		{"invalid syntax", "temperature >> 85", false},
		{"invalid field", "fake_field > 100", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"condition":"` + tt.condition + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/validate", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleValidate(w, req)

			var result map[string]interface{}
			if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
				t.Fatalf("failed to decode validate response: %v", err)
			}

			valid, ok := result["valid"].(bool)
			if !ok {
				t.Fatal("Response missing 'valid' field")
			}

			if valid != tt.wantValid {
				t.Errorf("Expected valid=%v, got %v", tt.wantValid, valid)
				if !tt.wantValid {
					t.Logf("Error: %v", result["error"])
				}
			}

			// Check that valid conditions include paraphrase
			if valid {
				paraphrase, hasParaphrase := result["paraphrase"].(string)
				if !hasParaphrase {
					t.Error("Valid condition should include paraphrase")
				} else if paraphrase == "" {
					t.Error("Paraphrase should not be empty for valid condition")
				} else {
					t.Logf("Paraphrase: %s", paraphrase)
				}
			}
		})
	}
}

// TestHandleGetFields checks that handleGetFields returns a non-empty list
// including common condition fields.
func TestHandleGetFields(t *testing.T) {
	server := &Server{
		configPath: "test.json",
		port:       "8081",
		config:     &alarm.AlarmConfig{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/fields", nil)
	w := httptest.NewRecorder()
	server.handleGetFields(w, req)

	var fields []string
	if err := json.NewDecoder(w.Body).Decode(&fields); err != nil {
		t.Fatalf("failed to decode fields: %v", err)
	}

	// Check for some expected fields
	expectedFields := []string{"temperature", "humidity", "pressure", "wind_speed", "lux"}
	for _, expected := range expectedFields {
		found := false
		for _, field := range fields {
			if field == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected field '%s' not found in result", expected)
		}
	}
}

// TestLoadContacts_ValidJSON checks that loadContacts parses a valid
// CONTACT_LIST JSON array.
func TestLoadContacts_ValidJSON(t *testing.T) {
	// Set up environment with valid contact list
	originalContactList := os.Getenv("CONTACT_LIST")

	validContacts := `[
		{"name": "John Doe", "email": "john@example.com", "sms": "+1234567890"},
		{"name": "Jane Smith", "email": "jane@example.com", "sms": "+0987654321"}
	]`
	_ = os.Setenv("CONTACT_LIST", validContacts)
	defer func() { _ = os.Setenv("CONTACT_LIST", originalContactList) }()

	server := &Server{}
	err := server.loadContacts()
	if err != nil {
		t.Fatalf("Expected no error for valid JSON, got: %v", err)
	}

	if len(server.contacts) != 2 {
		t.Errorf("Expected 2 contacts, got %d", len(server.contacts))
	}

	if server.contacts[0].Name != "John Doe" {
		t.Errorf("Expected first contact name 'John Doe', got '%s'", server.contacts[0].Name)
	}
}

// TestLoadContacts_InvalidJSON checks that loadContacts returns an error for
// malformed CONTACT_LIST JSON.
func TestLoadContacts_InvalidJSON(t *testing.T) {
	// Set up environment with invalid JSON
	originalContactList := os.Getenv("CONTACT_LIST")

	invalidJSON := `[{"name": "John", "email": "john@example.com", "sms": "+1234567890"` // Missing closing bracket
	_ = os.Setenv("CONTACT_LIST", invalidJSON)
	defer func() { _ = os.Setenv("CONTACT_LIST", originalContactList) }()

	server := &Server{}
	err := server.loadContacts()
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse CONTACT_LIST JSON") {
		t.Errorf("Expected parse error message, got: %v", err)
	}
}

// TestLoadContacts_ValidationWarnings checks that contacts with missing or
// malformed fields are still loaded (they only produce warnings).
func TestLoadContacts_ValidationWarnings(t *testing.T) {
	// Set up environment with contacts that should generate warnings
	originalContactList := os.Getenv("CONTACT_LIST")

	contactsWithWarnings := `[
		{"name": "", "email": "john@example.com", "sms": "+1234567890"},
		{"name": "Jane", "email": "", "sms": ""},
		{"name": "Bob", "email": "invalid-email", "sms": "+1234567890"},
		{"name": "Alice", "email": "alice@example.com", "sms": "1234567890"}
	]`
	_ = os.Setenv("CONTACT_LIST", contactsWithWarnings)
	defer func() { _ = os.Setenv("CONTACT_LIST", originalContactList) }()

	server := &Server{}
	err := server.loadContacts()
	if err != nil {
		t.Fatalf("Expected no error despite warnings, got: %v", err)
	}

	if len(server.contacts) != 4 {
		t.Errorf("Expected 4 contacts, got %d", len(server.contacts))
	}

	// The warnings would be logged but we can't easily test log output in this test
	// In a real scenario, these would generate warnings:
	// - Contact 1: empty name
	// - Contact 2: neither email nor SMS
	// - Contact 3: invalid email format
	// - Contact 4: SMS without + prefix
}

// TestHandleGetTags_InvalidJSON checks that malformed TAG_LIST JSON is ignored
// and alarm tags are still returned.
func TestHandleGetTags_InvalidJSON(t *testing.T) {
	// Set up environment with invalid TAG_LIST JSON
	originalTagList := os.Getenv("TAG_LIST")

	invalidJSON := `["temperature", "heat", "storm"` // Missing closing bracket
	_ = os.Setenv("TAG_LIST", invalidJSON)
	defer func() { _ = os.Setenv("TAG_LIST", originalTagList) }()

	config := &alarm.AlarmConfig{
		Alarms: []alarm.Alarm{
			{
				Name: "alarm1",
				Tags: []string{"temperature"},
			},
		},
	}

	server := &Server{
		configPath: "test.json",
		port:       "8081",
		config:     config,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	w := httptest.NewRecorder()
	server.handleGetTags(w, req)

	// Should still return tags from alarms even if TAG_LIST is invalid
	var tags []string
	if err := json.NewDecoder(w.Body).Decode(&tags); err != nil {
		t.Fatalf("failed to decode tags: %v", err)
	}

	if len(tags) != 1 || tags[0] != "temperature" {
		t.Errorf("Expected ['temperature'] when TAG_LIST is invalid, got %v", tags)
	}
}

// TestHandleGetTags_ValidationWarnings checks that empty TAG_LIST entries are
// dropped while tags that only warn (spaces, over 50 characters) are kept.
func TestHandleGetTags_ValidationWarnings(t *testing.T) {
	// Set up environment with TAG_LIST that should generate warnings
	originalTagList := os.Getenv("TAG_LIST")

	tagsWithWarnings := `["temperature", "", "tag with spaces", "verylongtagnameexceedingfiftycharacterslimitforatag", "normal-tag"]`
	_ = os.Setenv("TAG_LIST", tagsWithWarnings)
	defer func() { _ = os.Setenv("TAG_LIST", originalTagList) }()

	config := &alarm.AlarmConfig{
		Alarms: []alarm.Alarm{
			{
				Name: "alarm1",
				Tags: []string{"existing-tag"},
			},
		},
	}

	server := &Server{
		configPath: "test.json",
		port:       "8081",
		config:     config,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	w := httptest.NewRecorder()
	server.handleGetTags(w, req)

	var tags []string
	if err := json.NewDecoder(w.Body).Decode(&tags); err != nil {
		t.Fatalf("failed to decode tags: %v", err)
	}

	// Should include valid tags (excluding empty one) and be sorted alphabetically
	expectedTags := map[string]bool{
		"existing-tag":    true,
		"temperature":     true,
		"tag with spaces": true, // Still included despite warning
		"verylongtagnameexceedingfiftycharacterslimitforatag": true, // Still included despite warning
		"normal-tag": true,
	}

	for _, tag := range tags {
		if _, exists := expectedTags[tag]; exists {
			expectedTags[tag] = false // Mark as found
		}
	}

	for tag, notFound := range expectedTags {
		if notFound {
			t.Errorf("Expected tag '%s' not found in result", tag)
		}
	}

	// Check that tags are sorted alphabetically
	if len(tags) > 1 {
		for i := 1; i < len(tags); i++ {
			if tags[i-1] > tags[i] {
				t.Errorf("Tags not sorted alphabetically: %v", tags)
				break
			}
		}
	}

	// The warnings would be logged but we can't easily test log output in this test
	// In a real scenario, these would generate warnings:
	// - Tag 2: empty or whitespace-only
	// - Tag 3: contains spaces
	// - Tag 4: very long
}

// TestHandleGetTags_ValidJSON checks that a valid TAG_LIST is merged with
// alarm tags into one sorted, de-duplicated list.
func TestHandleGetTags_ValidJSON(t *testing.T) {
	// Set up environment with valid TAG_LIST JSON
	originalTagList := os.Getenv("TAG_LIST")

	validTags := `["temperature", "heat", "storm", "lightning"]`
	_ = os.Setenv("TAG_LIST", validTags)
	defer func() { _ = os.Setenv("TAG_LIST", originalTagList) }()

	config := &alarm.AlarmConfig{
		Alarms: []alarm.Alarm{
			{
				Name: "alarm1",
				Tags: []string{"temperature", "custom-tag"},
			},
		},
	}

	server := &Server{
		configPath: "test.json",
		port:       "8081",
		config:     config,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	w := httptest.NewRecorder()
	server.handleGetTags(w, req)

	var tags []string
	if err := json.NewDecoder(w.Body).Decode(&tags); err != nil {
		t.Fatalf("failed to decode tags: %v", err)
	}

	// Should include both alarm tags and predefined tags
	expectedTags := map[string]bool{
		"temperature": true,
		"custom-tag":  true,
		"heat":        true,
		"storm":       true,
		"lightning":   true,
	}

	for _, tag := range tags {
		if _, exists := expectedTags[tag]; exists {
			expectedTags[tag] = false // Mark as found
		}
	}

	for tag, notFound := range expectedTags {
		if notFound {
			t.Errorf("Expected tag '%s' not found in result", tag)
		}
	}

	// Check that tags are sorted alphabetically
	if len(tags) > 1 {
		for i := 1; i < len(tags); i++ {
			if tags[i-1] > tags[i] {
				t.Errorf("Tags not sorted alphabetically: %v", tags)
				break
			}
		}
	}
}

// TestHandleGetContacts_SortedAlphabetically checks that handleGetContacts
// returns contacts sorted by name.
func TestHandleGetContacts_SortedAlphabetically(t *testing.T) {
	// Create server with contacts in non-alphabetical order
	server := &Server{
		configPath: "test.json",
		port:       "8081",
		contacts: []Contact{
			{Name: "Charlie", Email: "charlie@example.com", SMS: "+1111111111"},
			{Name: "Alice", Email: "alice@example.com", SMS: "+2222222222"},
			{Name: "Bob", Email: "bob@example.com", SMS: "+3333333333"},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/contacts", nil)
	w := httptest.NewRecorder()
	server.handleGetContacts(w, req)

	var contacts []Contact
	if err := json.NewDecoder(w.Body).Decode(&contacts); err != nil {
		t.Fatalf("failed to decode contacts: %v", err)
	}

	// Check that contacts are sorted alphabetically by name
	if len(contacts) != 3 {
		t.Errorf("Expected 3 contacts, got %d", len(contacts))
	}

	expectedOrder := []string{"Alice", "Bob", "Charlie"}
	for i, contact := range contacts {
		if contact.Name != expectedOrder[i] {
			t.Errorf("Expected contact %d to be '%s', got '%s'", i, expectedOrder[i], contact.Name)
		}
	}
}
