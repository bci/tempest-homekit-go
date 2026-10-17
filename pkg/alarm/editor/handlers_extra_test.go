package editor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"tempest-homekit-go/pkg/alarm"
)

// TestCreateUpdateDeleteAlarm_Workflow drives create, rename-via-oldName
// update and delete through the handlers and checks the list after each step
// and the zero-alarm file left on disk.
func TestCreateUpdateDeleteAlarm_Workflow(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "alarms_editor_test_*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	// Start with empty config
	server := &Server{
		configPath: tmpfile.Name(),
		port:       "0",
		config:     &alarm.AlarmConfig{Alarms: []alarm.Alarm{}},
	}

	// Create alarm
	createBody := `{"name":"a1","condition":"temperature > 10","enabled":true,"channels":[{"type":"console","template":"t"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/alarms/create", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.handleCreateAlarm(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create alarm failed: code=%d body=%s", w.Code, w.Body.String())
	}

	// List and verify
	req = httptest.NewRequest(http.MethodGet, "/api/alarms", nil)
	w = httptest.NewRecorder()
	server.handleListAlarms(w, req)
	var list []alarm.Alarm
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "a1" {
		t.Fatalf("unexpected alarms after create: %#v", list)
	}

	// Update alarm - rename to a1-renamed
	updated := `{"name":"a1-renamed","condition":"temperature > 10","enabled":true,"channels":[{"type":"console","template":"updated"}]}`
	req = httptest.NewRequest(http.MethodPost, "/api/alarms/update?oldName=a1", strings.NewReader(updated))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.handleUpdateAlarm(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update alarm failed: code=%d body=%s", w.Code, w.Body.String())
	}

	// List and verify update applied
	req = httptest.NewRequest(http.MethodGet, "/api/alarms", nil)
	w = httptest.NewRecorder()
	server.handleListAlarms(w, req)
	list = nil
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode list after update: %v", err)
	}
	if len(list) != 1 || list[0].Name != "a1-renamed" {
		t.Fatalf("unexpected alarms after update: %#v", list)
	}

	// Delete alarm
	req = httptest.NewRequest(http.MethodPost, "/api/alarms/delete?name=a1-renamed", nil)
	w = httptest.NewRecorder()
	server.handleDeleteAlarm(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete alarm failed: code=%d body=%s", w.Code, w.Body.String())
	}

	// List and verify empty
	req = httptest.NewRequest(http.MethodGet, "/api/alarms", nil)
	w = httptest.NewRecorder()
	server.handleListAlarms(w, req)
	list = nil
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode list after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no alarms after delete, got: %#v", list)
	}

	// Ensure config file was written and contains zero alarms
	data, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	var cfg alarm.AlarmConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to unmarshal saved config: %v", err)
	}
	if len(cfg.Alarms) != 0 {
		t.Fatalf("expected saved config to have 0 alarms, got %d", len(cfg.Alarms))
	}
}

// TestHandleSaveConfig_InvalidConfig_ReturnsBadRequest checks that
// handleSaveConfig rejects a config that fails Validate (an alarm with no
// channels) with a non-200 status.
func TestHandleSaveConfig_InvalidConfig_ReturnsBadRequest(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "alarms_editor_test_save_*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	server := &Server{
		configPath: tmpfile.Name(),
		port:       "0",
		config:     &alarm.AlarmConfig{Alarms: []alarm.Alarm{}},
	}

	// Missing channels will make validation fail
	badCfg := `{"alarms":[{"name":"bad","condition":"temperature > 0","enabled":true}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/config/save", strings.NewReader(badCfg))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.handleSaveConfig(w, req)
	if w.Code == http.StatusOK {
		t.Fatalf("expected bad request for invalid config, got 200; body=%s", w.Body.String())
	}
}

// TestHandleGetEnvDefaults_ReturnsEnvValues checks that handleGetEnvDefaults
// reports MS365_TO_ADDRESS and SMS_TO_NUMBER as emailTo and smsTo.
func TestHandleGetEnvDefaults_ReturnsEnvValues(t *testing.T) {
	_ = os.Setenv("MS365_TO_ADDRESS", "foo@example.com")
	_ = os.Setenv("SMS_TO_NUMBER", "+15551234567")
	defer func() { _ = os.Unsetenv("MS365_TO_ADDRESS") }()
	defer func() { _ = os.Unsetenv("SMS_TO_NUMBER") }()

	server := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/env-defaults", nil)
	w := httptest.NewRecorder()
	server.handleGetEnvDefaults(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("env defaults handler returned status %d", w.Code)
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode env defaults: %v", err)
	}

	if result["emailTo"] != "foo@example.com" {
		t.Fatalf("unexpected emailTo: %v", result["emailTo"])
	}
	if result["smsTo"] != "+15551234567" {
		t.Fatalf("unexpected smsTo: %v", result["smsTo"])
	}
}

// TestHandleGetContacts_ReturnsContactList checks that contacts loaded from
// the CONTACT_LIST environment variable are returned with all fields, sorted
// by name.
func TestHandleGetContacts_ReturnsContactList(t *testing.T) {
	// Set up test contact list
	contactListJSON := `[
		{"name": "John Doe", "email": "john@example.com", "sms": "+15551234567"},
		{"name": "Jane Smith", "email": "jane@example.com", "sms": "+15559876543"}
	]`
	_ = os.Setenv("CONTACT_LIST", contactListJSON)
	defer func() { _ = os.Unsetenv("CONTACT_LIST") }()

	server := &Server{}
	// Load contacts
	if err := server.loadContacts(); err != nil {
		t.Fatalf("failed to load contacts: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/contacts", nil)
	w := httptest.NewRecorder()
	server.handleGetContacts(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("contacts handler returned status %d", w.Code)
	}

	var result []Contact
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode contacts: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 contacts, got %d", len(result))
	}

	if result[0].Name != "Jane Smith" || result[0].Email != "jane@example.com" || result[0].SMS != "+15559876543" {
		t.Fatalf("unexpected first contact: %+v", result[0])
	}

	if result[1].Name != "John Doe" || result[1].Email != "john@example.com" || result[1].SMS != "+15551234567" {
		t.Fatalf("unexpected second contact: %+v", result[1])
	}
}

// TestHandleGetContacts_EmptyList checks that handleGetContacts returns an
// empty JSON array when no CONTACT_LIST is configured.
func TestHandleGetContacts_EmptyList(t *testing.T) {
	// No CONTACT_LIST environment variable set
	server := &Server{}
	// Load contacts (should be empty)
	if err := server.loadContacts(); err != nil {
		t.Fatalf("failed to load contacts: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/contacts", nil)
	w := httptest.NewRecorder()
	server.handleGetContacts(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("contacts handler returned status %d", w.Code)
	}

	var result []Contact
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode contacts: %v", err)
	}

	if len(result) != 0 {
		t.Fatalf("expected empty contact list, got %d contacts", len(result))
	}
}
