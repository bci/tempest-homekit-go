package alarm

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestWebhookConfiguration tests webhook notification by sending a test message.
//
// The target URL comes from the TEST_WEBHOOK_URL environment variable, not
// from the alarm config. alarmsJSON is the alarm config input accepted by
// [LoadAlarmConfig] (used to build the notifier through the real factory);
// stationName fills {{station}}. It prints the request and the expanded JSON
// body, then POSTs it. It returns an error if TEST_WEBHOOK_URL is unset, the
// config fails to load, or the request fails or gets a non-2xx response.
func TestWebhookConfiguration(alarmsJSON, stationName string) error {
	fmt.Println("Testing webhook notification output...")
	fmt.Println()

	// Get test URL from environment
	testURL := os.Getenv("TEST_WEBHOOK_URL")
	if testURL == "" {
		return fmt.Errorf("TEST_WEBHOOK_URL environment variable not set")
	}

	// Load alarm configuration (uses factory for real delivery path)
	config, err := LoadAlarmConfig(alarmsJSON)
	if err != nil {
		return fmt.Errorf("failed to load alarm configuration: %w", err)
	}

	// Create webhook notifier using factory
	factory := NewNotifierFactory(config)
	notifier, err := factory.GetNotifier("webhook")
	if err != nil {
		return fmt.Errorf("failed to create webhook notifier: %w", err)
	}

	// Create test alarm
	testAlarm := &Alarm{
		Name:        "Webhook Test",
		Description: "Test webhook notification output",
		Enabled:     true,
	}

	// Create test channel with webhook configuration
	testChannel := &Channel{
		Type: "webhook",
		Webhook: &WebhookConfig{
			URL:         testURL,
			Method:      "POST",
			Headers:     map[string]string{"Content-Type": "application/json", "User-Agent": "Tempest-HomeKit-Webhook-Test"},
			Body:        `{"alarm":{"name":"{{alarm_name}}","description":"{{alarm_description}}"},"station":"{{station}}","timestamp":"{{timestamp}}","sensors":{"temperature_c":{{temperature}},"humidity":{{humidity}},"pressure_mb":{{pressure}}}}`,
			ContentType: "application/json",
		},
	}

	// Create test observation
	testObs := &weather.Observation{
		Timestamp:        time.Now().Unix(),
		AirTemperature:   20.0,
		RelativeHumidity: 50.0,
		WindAvg:          5.0,
		StationPressure:  1013.25,
	}

	// Send test notification
	fmt.Printf("Sending test webhook to: %s\n", testURL)
	fmt.Println("Request details:")
	fmt.Printf("  Method: %s\n", testChannel.Webhook.Method)
	fmt.Printf("  Content-Type: %s\n", testChannel.Webhook.ContentType)
	fmt.Printf("  Headers: %v\n", testChannel.Webhook.Headers)
	fmt.Println()

	// Expand the body template to show what will be sent
	expandedBody := expandTemplate(testChannel.Webhook.Body, testAlarm, testObs, stationName)
	fmt.Println("Request body (expanded template):")
	fmt.Println("─────────────────────────────────────────────────────────────")
	fmt.Println(expandedBody)
	fmt.Println("─────────────────────────────────────────────────────────────")
	fmt.Println()

	// Send test notification
	if err = notifier.Send(testAlarm, testChannel, testObs, stationName); err != nil {
		return fmt.Errorf("failed to send test notification: %w", err)
	}

	fmt.Println("✅ Webhook notification test completed successfully!")
	fmt.Println("   The webhook was sent to the configured URL.")

	return nil
}

// RunWebhookTest is a convenience function that wraps TestWebhookConfiguration and exits.
// It never returns: it calls os.Exit(0) on success and log.Fatalf on failure.
func RunWebhookTest(alarmsJSON, stationName string) {
	if err := TestWebhookConfiguration(alarmsJSON, stationName); err != nil {
		log.Fatalf("Webhook test failed: %v", err)
	}
	os.Exit(0)
}

// TestWebhookConfigurationWithServer creates a test HTTP server and tests webhook delivery.
//
// It starts a local httptest server that prints and records each request,
// points TEST_WEBHOOK_URL at it (left set in the process environment
// afterwards), runs [TestWebhookConfiguration], and prints a summary of the
// last JSON body received. It returns an error if the webhook test fails or
// the server received no JSON request.
func TestWebhookConfigurationWithServer(alarmsJSON, stationName string) error {
	fmt.Println("Testing webhook notification with local test server...")
	fmt.Println()

	// Create a test HTTP server
	var receivedRequests []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log the request
		fmt.Printf("📨 Received webhook request:\n")
		fmt.Printf("  Method: %s\n", r.Method)
		fmt.Printf("  URL: %s\n", r.URL.String())
		fmt.Printf("  Content-Type: %s\n", r.Header.Get("Content-Type"))
		fmt.Printf("  User-Agent: %s\n", r.Header.Get("User-Agent"))
		fmt.Println()

		// Read the body
		body, _ := io.ReadAll(r.Body)
		if len(body) > 0 {
			fmt.Println("  Body:")
			fmt.Println("  ─────────────────────────────────────────────────────────────")
			fmt.Println("  " + string(body))
			fmt.Println("  ─────────────────────────────────────────────────────────────")
			fmt.Println()

			// Try to parse as JSON for pretty printing
			var jsonData map[string]interface{}
			if err := json.Unmarshal(body, &jsonData); err == nil {
				fmt.Println("  Parsed JSON:")
				prettyJSON, _ := json.MarshalIndent(jsonData, "  ", "  ")
				fmt.Println("  " + strings.ReplaceAll(string(prettyJSON), "\n", "\n  "))
				fmt.Println()

				receivedRequests = append(receivedRequests, jsonData)
			}
		}

		// Send success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok","message":"Webhook received successfully"}`)); err != nil {
			log.Printf("failed to write webhook test response: %v", err)
		}
	}))
	defer server.Close()

	fmt.Printf("🖥️  Test server started at: %s\n", server.URL)
	fmt.Println()

	// Set the test URL environment variable
	_ = os.Setenv("TEST_WEBHOOK_URL", server.URL)

	// Run the webhook test
	if err := TestWebhookConfiguration(alarmsJSON, stationName); err != nil {
		return err
	}

	// Verify we received the request
	if len(receivedRequests) == 0 {
		return fmt.Errorf("no webhook requests were received by the test server")
	}

	fmt.Printf("✅ Test server received %d webhook request(s)\n", len(receivedRequests))

	// Show summary of received data
	if len(receivedRequests) > 0 {
		lastRequest := receivedRequests[len(receivedRequests)-1]
		fmt.Println("📊 Last request summary:")
		if alarm, ok := lastRequest["alarm"].(map[string]interface{}); ok {
			if name, ok := alarm["name"].(string); ok {
				fmt.Printf("  Alarm Name: %s\n", name)
			}
			if desc, ok := alarm["description"].(string); ok {
				fmt.Printf("  Description: %s\n", desc)
			}
		}
		if station, ok := lastRequest["station"].(string); ok {
			fmt.Printf("  Station: %s\n", station)
		}
		if sensors, ok := lastRequest["sensors"].(map[string]interface{}); ok {
			fmt.Printf("  Sensor Count: %d\n", len(sensors))
		}
		fmt.Println()
	}

	return nil
}
