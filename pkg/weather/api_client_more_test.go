package weather

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGetForecast checks that GetForecast decodes daily forecast periods.
func TestGetForecast(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// simple forecast response
		fr := ForecastResponse{
			Forecast: struct {
				Daily []ForecastPeriod `json:"daily"`
			}{Daily: []ForecastPeriod{{Time: 1, AirTemperature: 20.0}}},
			CurrentConditions: ForecastPeriod{Time: 1, AirTemperature: 20.0},
		}
		b, _ := json.Marshal(fr)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	orig := http.DefaultTransport
	http.DefaultTransport = &helperRoundTripper{base: orig, rewriteTo: srv.Listener.Addr().String()}
	defer func() { http.DefaultTransport = orig }()

	f, err := GetForecast(1, "token")
	if err != nil {
		t.Fatalf("GetForecast error: %v", err)
	}
	if f == nil || len(f.Forecast.Daily) != 1 {
		t.Fatalf("unexpected forecast: %+v", f)
	}
}

// TestGetStationStatus_Non200 checks that GetStationStatus fails on an HTTP 500.
func TestGetStationStatus_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	orig := http.DefaultTransport
	http.DefaultTransport = &helperRoundTripper{base: orig, rewriteTo: srv.Listener.Addr().String()}
	defer func() { http.DefaultTransport = orig }()

	_, err := GetStationStatus(123, "debug")
	if err == nil {
		t.Fatalf("expected error for non-200 station status")
	}
}

// TestGetObservationFromURL_Non200 checks that GetObservationFromURL fails on an HTTP 404.
func TestGetObservationFromURL_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	orig := http.DefaultTransport
	http.DefaultTransport = &helperRoundTripper{base: orig, rewriteTo: srv.Listener.Addr().String()}
	defer func() { http.DefaultTransport = orig }()

	_, err := GetObservationFromURL("http://example/obs")
	if err == nil {
		t.Fatalf("expected error from GetObservationFromURL when server returns non-200")
	}
}
