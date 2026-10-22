package weather

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// overrideDefaultTransportToServer temporarily overrides http.DefaultTransport to
// route requests to the provided test server. It returns a function to restore
// the transport.
func overrideDefaultTransportToServer(srv *httptest.Server) func() {
	old := http.DefaultTransport
	target, _ := url.Parse(srv.URL)

	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		// rewrite scheme and host to point at test server
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		return old.RoundTrip(req)
	})

	return func() { http.DefaultTransport = old }
}

// roundTripperFunc adapts a function to the http.RoundTripper interface.
type roundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip calls f.
func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestGetStationDetails_Success checks that GetStationDetails decodes a station by ID.
func TestGetStationDetails_Success(t *testing.T) {
	// Create a mock StationDetailsResponse
	resp := StationDetailsResponse{
		Stations: []Station{{StationID: 111, Name: "Mock", StationName: "Mock"}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	restore := overrideDefaultTransportToServer(srv)
	defer restore()

	details, err := GetStationDetails(111, "token")
	if err != nil {
		t.Fatalf("GetStationDetails failed: %v", err)
	}
	if details == nil || details.StationID != 111 {
		t.Fatalf("unexpected station details: %+v", details)
	}
}

// TestGetHistoricalObservationsWithProgress_Success checks that observations are returned and the progress callback is invoked.
func TestGetHistoricalObservationsWithProgress_Success(t *testing.T) {
	// Mock station details response to return a device of type ST
	stationResp := StationDetailsResponse{
		Stations: []Station{{StationID: 222, Name: "S", StationName: "S", Devices: []Device{{DeviceID: 999, DeviceType: "ST"}}}},
	}

	// Mock historical responses for device endpoint
	historical := HistoricalResponse{
		Status: map[string]interface{}{"status_code": float64(0)},
		Obs: [][]interface{}{
			{float64(time.Now().Unix()), 0.0, 0.0, 0.0, 0.0, 0.0, 1013.0, 20.0, 50.0, 100.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 3.8, 60.0},
		},
	}

	mux := http.NewServeMux()
	// Match paths that may include the BaseURL prefix (e.g., /swd/rest/...)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case len(p) >= len("/stations/222") && p[len(p)-len("/stations/222"):] == "/stations/222":
			_ = json.NewEncoder(w).Encode(stationResp)
		case len(p) >= len("/observations/device/999") && p[len(p)-len("/observations/device/999"):] == "/observations/device/999":
			_ = json.NewEncoder(w).Encode(historical)
		default:
			http.NotFound(w, r)
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	restore := overrideDefaultTransportToServer(srv)
	defer restore()

	// Collect progress callbacks
	progressCalls := 0
	cb := func(currentStep, totalSteps int, description string) {
		progressCalls++
	}

	results, err := GetHistoricalObservationsWithProgress(222, "token", "debug", cb, 100)
	if err != nil {
		t.Fatalf("GetHistoricalObservationsWithProgress failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected some historical observations, got 0")
	}
	if progressCalls == 0 {
		t.Fatalf("expected progress callbacks to be invoked")
	}
}
