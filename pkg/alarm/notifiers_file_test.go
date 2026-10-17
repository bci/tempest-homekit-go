package alarm

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestCSVNotifier_appendToCSVFile checks that appendToCSVFile writes a header row only
// while the file is empty, choosing the eleven-column header for a message containing
// commas and the timestamp/message header otherwise, and appends rows on later writes.
func TestCSVNotifier_appendToCSVFile(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "csv_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	notifier := &CSVNotifier{}

	tests := []struct {
		name             string
		message          string
		expectedHeaders  []string
		expectedRowCount int
		description      string
	}{
		{
			name:             "multi-column format",
			message:          "High Temp,High temperature detected,30.5,75.0,1013.25,5.5,45000,8,2.5,ALARM: High Temp triggered",
			expectedHeaders:  []string{"timestamp", "alarm_name", "alarm_description", "temperature", "humidity", "pressure", "wind_speed", "lux", "uv", "rain_daily", "message"},
			expectedRowCount: 1,
			description:      "Test multi-column CSV format with all sensor data",
		},
		{
			name:             "simple format",
			message:          "Simple alarm message",
			expectedHeaders:  []string{"timestamp", "message"},
			expectedRowCount: 1,
			description:      "Test simple CSV format with just timestamp and message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(tempDir, fmt.Sprintf("test_%s.csv", tt.name))

			// First write - should create file with headers
			err := notifier.appendToCSVFile(filePath, tt.message, 30)
			if err != nil {
				t.Fatalf("Failed to write CSV file: %v", err)
			}

			// Verify file exists
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Fatalf("CSV file was not created")
			}

			// Read and verify content
			file, err := os.Open(filePath)
			if err != nil {
				t.Fatalf("Failed to open CSV file: %v", err)
			}
			defer func() { _ = file.Close() }()

			reader := csv.NewReader(file)
			records, err := reader.ReadAll()
			if err != nil {
				t.Fatalf("Failed to read CSV file: %v", err)
			}

			// Check header row
			if len(records) < 1 {
				t.Fatalf("CSV file should have at least header row")
			}

			headers := records[0]
			if len(headers) != len(tt.expectedHeaders) {
				t.Errorf("Expected %d headers, got %d", len(tt.expectedHeaders), len(headers))
			}

			for i, expected := range tt.expectedHeaders {
				if i < len(headers) && headers[i] != expected {
					t.Errorf("Header %d: expected %q, got %q", i, expected, headers[i])
				}
			}

			// Check data rows
			dataRows := len(records) - 1 // Subtract header row
			if dataRows != tt.expectedRowCount {
				t.Errorf("Expected %d data rows, got %d", tt.expectedRowCount, dataRows)
			}

			// Second write - should append without headers
			err = notifier.appendToCSVFile(filePath, tt.message, 30)
			if err != nil {
				t.Fatalf("Failed to append to CSV file: %v", err)
			}

			// Re-read file
			if _, err := file.Seek(0, 0); err != nil {
				t.Fatalf("failed to seek file: %v", err)
			}
			reader = csv.NewReader(file)
			records, err = reader.ReadAll()
			if err != nil {
				t.Fatalf("Failed to re-read CSV file: %v", err)
			}

			// Should now have header + 2 data rows
			expectedTotalRows := 1 + (tt.expectedRowCount * 2)
			if len(records) != expectedTotalRows {
				t.Errorf("Expected %d total rows after append, got %d", expectedTotalRows, len(records))
			}
		})
	}
}

// TestCSVNotifier_appendToCSVFile_Rotation checks that a CSV file older than maxDays is
// renamed to a ".<timestamp>.bak" backup and a fresh file is started.
func TestCSVNotifier_appendToCSVFile_Rotation(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "csv_rotation_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	notifier := &CSVNotifier{}
	filePath := filepath.Join(tempDir, "test.csv")

	// Create file with old timestamp
	err = notifier.appendToCSVFile(filePath, "test message", 1) // 1 day max
	if err != nil {
		t.Fatalf("Failed to create CSV file: %v", err)
	}

	// Modify file timestamp to be 2 days old
	oldTime := time.Now().AddDate(0, 0, -2)
	err = os.Chtimes(filePath, oldTime, oldTime)
	if err != nil {
		t.Fatalf("Failed to change file timestamp: %v", err)
	}

	// Write again - should rotate the file
	err = notifier.appendToCSVFile(filePath, "new message", 1)
	if err != nil {
		t.Fatalf("Failed to write after rotation: %v", err)
	}

	// Check that backup file exists
	backupPattern := filePath + ".*.bak"
	matches, err := filepath.Glob(backupPattern)
	if err != nil {
		t.Fatalf("Failed to check for backup files: %v", err)
	}

	if len(matches) == 0 {
		t.Error("Expected backup file to be created during rotation")
	}

	// Check that new file exists and has content
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("New CSV file should exist after rotation")
	}
}

// TestJSONNotifier_appendToJSONFile checks that appendToJSONFile keeps the file as one
// valid, newline-delimited JSON array and appends a record on each write.
func TestJSONNotifier_appendToJSONFile(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "json_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	notifier := &JSONNotifier{}

	tests := []struct {
		name        string
		message     string
		description string
	}{
		{
			name:        "simple message",
			message:     `{"alarm": "test", "value": 25.5}`,
			description: "Test JSON file writing with simple message",
		},
		{
			name:        "complex message",
			message:     `{"timestamp": "2024-01-01 12:00:00", "alarm": {"name": "High Temp", "condition": "temperature > 30"}, "sensors": {"temperature": 32.5, "humidity": 65}}`,
			description: "Test JSON file writing with complex nested message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(tempDir, fmt.Sprintf("test_%s.json", tt.name))

			// First write - should create file with array
			err := notifier.appendToJSONFile(filePath, tt.message, 30)
			if err != nil {
				t.Fatalf("Failed to write JSON file: %v", err)
			}

			// Verify file exists
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Fatalf("JSON file was not created")
			}

			// Read and verify content
			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read JSON file: %v", err)
			}

			contentStr := string(content)
			if !strings.HasPrefix(contentStr, "[\n") || !strings.HasSuffix(strings.TrimSpace(contentStr), "\n]") {
				t.Errorf("JSON file should be a valid JSON array, got: %s", contentStr)
			}

			// Parse as JSON to verify structure
			var jsonData []interface{}
			err = json.Unmarshal(content, &jsonData)
			if err != nil {
				t.Fatalf("JSON file content is not valid JSON: %v", err)
			}

			if len(jsonData) != 1 {
				t.Errorf("Expected 1 JSON record, got %d", len(jsonData))
			}

			// Second write - should append to array
			err = notifier.appendToJSONFile(filePath, tt.message, 30)
			if err != nil {
				t.Fatalf("Failed to append to JSON file: %v", err)
			}

			// Re-read and verify
			content, err = os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to re-read JSON file: %v", err)
			}

			err = json.Unmarshal(content, &jsonData)
			if err != nil {
				t.Fatalf("JSON file content after append is not valid JSON: %v", err)
			}

			if len(jsonData) != 2 {
				t.Errorf("Expected 2 JSON records after append, got %d", len(jsonData))
			}
		})
	}
}

// TestJSONNotifier_appendToJSONFile_Rotation checks that a JSON file older than maxDays is
// backed up and replaced by a new array holding only the new record.
func TestJSONNotifier_appendToJSONFile_Rotation(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "json_rotation_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	notifier := &JSONNotifier{}
	filePath := filepath.Join(tempDir, "test.json")

	// Create file with old timestamp
	err = notifier.appendToJSONFile(filePath, `{"test": "data"}`, 1) // 1 day max
	if err != nil {
		t.Fatalf("Failed to create JSON file: %v", err)
	}

	// Modify file timestamp to be 2 days old
	oldTime := time.Now().AddDate(0, 0, -2)
	err = os.Chtimes(filePath, oldTime, oldTime)
	if err != nil {
		t.Fatalf("Failed to change file timestamp: %v", err)
	}

	// Write again - should rotate the file
	err = notifier.appendToJSONFile(filePath, `{"new": "data"}`, 1)
	if err != nil {
		t.Fatalf("Failed to write after rotation: %v", err)
	}

	// Check that backup file exists
	backupPattern := filePath + ".*.bak"
	matches, err := filepath.Glob(backupPattern)
	if err != nil {
		t.Fatalf("Failed to check for backup files: %v", err)
	}

	if len(matches) == 0 {
		t.Error("Expected backup file to be created during rotation")
	}

	// Check that new file exists and has content
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("New JSON file should exist after rotation")
	}

	// Verify new file contains only the new record
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read rotated JSON file: %v", err)
	}

	var jsonData []interface{}
	err = json.Unmarshal(content, &jsonData)
	if err != nil {
		t.Fatalf("Rotated JSON file content is not valid JSON: %v", err)
	}

	if len(jsonData) != 1 {
		t.Errorf("Expected 1 JSON record in rotated file, got %d", len(jsonData))
	}
}

// TestCSVNotifier_Send checks that CSVNotifier.Send expands the channel's message
// template into one multi-column row under the expected header, with {{message}}
// becoming "ALARM: <name> triggered".
func TestCSVNotifier_Send(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "csv_send_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	notifier := &CSVNotifier{}

	alarm := &Alarm{
		Name:        "Test Alarm",
		Description: "Test alarm for CSV",
		Condition:   "temperature > 25",
	}

	channel := &Channel{
		Type: "csv",
		CSV: &CSVConfig{
			Path:    filepath.Join(tempDir, "alarms.csv"),
			MaxDays: 30,
			Message: "{{alarm_name}},{{alarm_description}},{{temperature}},{{humidity}},{{pressure}},{{wind_speed}},{{lux}},{{uv}},{{rain_daily}},{{message}}",
		},
	}

	obs := &weather.Observation{
		AirTemperature:       30.5,
		RelativeHumidity:     75.0,
		StationPressure:      1013.25,
		WindAvg:              5.5,
		WindGust:             8.5,
		WindDirection:        180,
		Illuminance:          45000,
		UV:                   8,
		RainAccumulated:      2.5,
		LightningStrikeCount: 5,
		LightningStrikeAvg:   10.5,
		Timestamp:            time.Now().Unix(),
	}

	err = notifier.Send(alarm, channel, obs, "Test Station")
	if err != nil {
		t.Fatalf("CSVNotifier.Send() failed: %v", err)
	}

	// Verify file was created and has correct content
	if _, err := os.Stat(channel.CSV.Path); os.IsNotExist(err) {
		t.Fatal("CSV file was not created")
	}

	file, err := os.Open(channel.CSV.Path)
	if err != nil {
		t.Fatalf("Failed to open CSV file: %v", err)
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	// Should have header + 1 data row
	if len(records) != 2 {
		t.Errorf("Expected 2 rows (header + data), got %d", len(records))
	}

	// Check headers
	expectedHeaders := []string{"timestamp", "alarm_name", "alarm_description", "temperature", "humidity", "pressure", "wind_speed", "lux", "uv", "rain_daily", "message"}
	headers := records[0]
	if len(headers) != len(expectedHeaders) {
		t.Errorf("Expected %d headers, got %d", len(expectedHeaders), len(headers))
	}

	for i, expected := range expectedHeaders {
		if i < len(headers) && headers[i] != expected {
			t.Errorf("Header %d: expected %q, got %q", i, expected, headers[i])
		}
	}

	// Check data row has expected values
	dataRow := records[1]
	if len(dataRow) != len(expectedHeaders) {
		t.Errorf("Expected %d data columns, got %d", len(expectedHeaders), len(dataRow))
	}

	// Check specific values
	if dataRow[1] != "Test Alarm" {
		t.Errorf("Expected alarm name 'Test Alarm', got %q", dataRow[1])
	}
	if dataRow[2] != "Test alarm for CSV" {
		t.Errorf("Expected alarm description 'Test alarm for CSV', got %q", dataRow[2])
	}
	if dataRow[3] != "30.5" {
		t.Errorf("Expected temperature '30.5', got %q", dataRow[3])
	}
	if dataRow[10] != "ALARM: Test Alarm triggered" {
		t.Errorf("Expected message 'ALARM: Test Alarm triggered', got %q", dataRow[10])
	}
}

// TestJSONNotifier_Send checks that JSONNotifier.Send writes a record with a timestamp
// and a "message" string that itself parses as the expanded JSON template.
func TestJSONNotifier_Send(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "json_send_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	notifier := &JSONNotifier{}

	alarm := &Alarm{
		Name:        "Test Alarm",
		Description: "Test alarm for JSON",
		Condition:   "temperature > 25",
	}

	channel := &Channel{
		Type: "json",
		JSON: &JSONConfig{
			Path:    filepath.Join(tempDir, "alarms.json"),
			MaxDays: 30,
			Message: `{"timestamp": "{{timestamp}}", "alarm": {"name": "{{alarm_name}}", "description": "{{alarm_description}}"}, "sensors": {"temperature": {{temperature}}, "humidity": {{humidity}}}}`,
		},
	}

	obs := &weather.Observation{
		AirTemperature:   30.5,
		RelativeHumidity: 75.0,
		Timestamp:        time.Now().Unix(),
	}

	err = notifier.Send(alarm, channel, obs, "Test Station")
	if err != nil {
		t.Fatalf("JSONNotifier.Send() failed: %v", err)
	}

	// Verify file was created and has correct content
	if _, err := os.Stat(channel.JSON.Path); os.IsNotExist(err) {
		t.Fatal("JSON file was not created")
	}

	content, err := os.ReadFile(channel.JSON.Path)
	if err != nil {
		t.Fatalf("Failed to read JSON file: %v", err)
	}

	// Parse JSON array
	var jsonData []map[string]interface{}
	err = json.Unmarshal(content, &jsonData)
	if err != nil {
		t.Fatalf("JSON file content is not valid: %v\nContent: %s", err, string(content))
	}

	if len(jsonData) != 1 {
		t.Fatalf("Expected 1 JSON record, got %d\nContent: %s", len(jsonData), string(content))
	}

	record := jsonData[0]

	// Check timestamp exists
	if _, exists := record["timestamp"]; !exists {
		t.Errorf("JSON record should contain timestamp field\nContent: %s", string(content))
	}

	// Check message field contains the JSON string
	messageStr, ok := record["message"].(string)
	if !ok {
		t.Fatalf("JSON record should contain message string\nContent: %s", string(content))
	}

	// Parse the message string as JSON
	var messageData map[string]interface{}
	err = json.Unmarshal([]byte(messageStr), &messageData)
	if err != nil {
		t.Fatalf("Message field is not valid JSON: %v\nMessage: %s", err, messageStr)
	}

	// Check alarm object within the message
	alarmData, ok := messageData["alarm"].(map[string]interface{})
	if !ok {
		t.Fatalf("Message should contain alarm object\nMessage: %s", messageStr)
	}

	if alarmData["name"] != "Test Alarm" {
		t.Errorf("Expected alarm name 'Test Alarm', got %v", alarmData["name"])
	}

	if alarmData["description"] != "Test alarm for JSON" {
		t.Errorf("Expected alarm description 'Test alarm for JSON', got %v", alarmData["description"])
	}

	// Check sensors object within the message
	sensorsData, ok := messageData["sensors"].(map[string]interface{})
	if !ok {
		t.Fatalf("Message should contain sensors object\nMessage: %s", messageStr)
	}

	if sensorsData["temperature"] != 30.5 {
		t.Errorf("Expected temperature 30.5, got %v", sensorsData["temperature"])
	}

	if sensorsData["humidity"] != 75.0 {
		t.Errorf("Expected humidity 75.0, got %v", sensorsData["humidity"])
	}
}

// TestCSVNotifier_Send_ErrorHandling checks that CSVNotifier.Send errors with "CSV
// configuration missing for channel" when the channel has no CSV block.
func TestCSVNotifier_Send_ErrorHandling(t *testing.T) {
	notifier := &CSVNotifier{}

	alarm := &Alarm{Name: "Test Alarm"}
	channel := &Channel{
		Type: "csv",
		CSV:  nil, // Missing CSV config
	}
	obs := &weather.Observation{}

	err := notifier.Send(alarm, channel, obs, "Test Station")
	if err == nil {
		t.Error("Expected error for missing CSV config")
	}
	if err.Error() != "CSV configuration missing for channel" {
		t.Errorf("Expected 'CSV configuration missing' error, got: %v", err)
	}
}

// TestJSONNotifier_Send_ErrorHandling checks that JSONNotifier.Send errors with "JSON
// configuration missing for channel" when the channel has no JSON block.
func TestJSONNotifier_Send_ErrorHandling(t *testing.T) {
	notifier := &JSONNotifier{}

	alarm := &Alarm{Name: "Test Alarm"}
	channel := &Channel{
		Type: "json",
		JSON: nil, // Missing JSON config
	}
	obs := &weather.Observation{}

	err := notifier.Send(alarm, channel, obs, "Test Station")
	if err == nil {
		t.Error("Expected error for missing JSON config")
	}
	if err.Error() != "JSON configuration missing for channel" {
		t.Errorf("Expected 'JSON configuration missing' error, got: %v", err)
	}
}
