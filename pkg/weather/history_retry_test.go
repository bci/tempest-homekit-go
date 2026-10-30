package weather

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetAllHistoricalObservations_NetworkErrorsDoNotPanic checks that when every
// observation request fails at the transport level (no HTTP response at all), the
// history loader gives up on that day instead of dereferencing a nil response.
// Before the 2026-10-16 fix this panicked in io.ReadAll(resp.Body).
func TestGetAllHistoricalObservations_NetworkErrorsDoNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/stations/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"stations":[{"station_id":1,"name":"T","devices":[{"device_id":42,"device_type":"ST"}]}]}`))
			return
		}
		// Observation requests: drop the connection so http.Get returns an error.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("test server does not support hijacking")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack failed: %v", err)
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	restore := overrideDefaultTransportToServer(srv)
	defer restore()

	obs, err := GetAllHistoricalObservations(1, "tok", "info", 1, "", 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("expected no error when a day simply has no data, got %v", err)
	}
	if len(obs) != 0 {
		t.Fatalf("expected no observations, got %d", len(obs))
	}
}

// TestGetAllHistoricalObservations_Non200CountsOnce checks that a non-200 response for
// a day is counted as one empty day, so three failing days are attempted before the
// loader stops (it used to count each failure twice and stop after two days).
func TestGetAllHistoricalObservations_Non200CountsOnce(t *testing.T) {
	obsRequests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/stations/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"stations":[{"station_id":1,"name":"T","devices":[{"device_id":42,"device_type":"ST"}]}]}`))
			return
		}
		obsRequests++
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	restore := overrideDefaultTransportToServer(srv)
	defer restore()

	if _, err := GetAllHistoricalObservations(1, "tok", "info", 10, "", 0, 0, 0, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obsRequests != 3 {
		t.Fatalf("expected 3 day requests before giving up after 3 consecutive failures, got %d", obsRequests)
	}
}
