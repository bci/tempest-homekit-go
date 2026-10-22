package weather

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// rt is a small transport helper to route any request to the test server: an http.RoundTripper backed by a plain function.
type rt func(*http.Request) (*http.Response, error)

// RoundTrip calls f.
func (f rt) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// overrideTransportToTestServer replaces http.DefaultTransport with one that
// sends every request to srv, keeping the path and query, and returns a
// function that restores the original transport.
func overrideTransportToTestServer(srv *httptest.Server) func() {
	old := http.DefaultTransport
	target, _ := url.Parse(srv.URL)

	http.DefaultTransport = rt(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		return old.RoundTrip(req)
	})

	return func() { http.DefaultTransport = old }
}

// TestGetStations_Non200 checks that GetStations fails on an HTTP 500.
func TestGetStations_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	restore := overrideTransportToTestServer(srv)
	defer restore()

	_, err := GetStations("token")
	if err == nil {
		t.Fatalf("expected error when stations endpoint returns non-200")
	}
}

// TestGetHistoricalObservationsWithProgress_StationDetailsError checks that a failed station details lookup is returned as an error.
func TestGetHistoricalObservationsWithProgress_StationDetailsError(t *testing.T) {
	// Server returns 500 for station details
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	restore := overrideTransportToTestServer(srv)
	defer restore()

	_, err := GetHistoricalObservationsWithProgress(9999, "token", "", nil, 100)
	if err == nil {
		t.Fatalf("expected error when station details cannot be fetched")
	}
}

// TestParseDeviceObservations_SkipsIncomplete checks that rows shorter than 18 values are dropped.
func TestParseDeviceObservations_SkipsIncomplete(t *testing.T) {
	// Provide an incomplete observation (less than 18 elements)
	incomplete := [][]interface{}{
		{float64(1234567890), 1.0, 2.0},
	}

	out := parseDeviceObservations(incomplete)
	if len(out) != 0 {
		t.Fatalf("expected incomplete observations to be skipped, got %d entries", len(out))
	}
}
