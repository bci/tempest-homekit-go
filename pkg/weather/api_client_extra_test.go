package weather

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// helperRoundTripper rewrites requests to the test server URL: it sends each request to rewriteTo (host:port) over plain HTTP via base.
type helperRoundTripper struct {
	base      http.RoundTripper
	rewriteTo string
}

// RoundTrip points req at the test server and forwards it to the base transport.
func (h *helperRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the request URL to the test server
	req.URL.Scheme = "http"
	req.URL.Host = h.rewriteTo
	return h.base.RoundTrip(req)
}

// TestGetStations checks that GetStations decodes the station list.
func TestGetStations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// simple stations response
		resp := StationsResponse{Stations: []Station{{StationID: 1, Name: "Test", StationName: "Test"}}}
		b, _ := json.Marshal(resp)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	// Swap default transport
	orig := http.DefaultTransport
	http.DefaultTransport = &helperRoundTripper{base: orig, rewriteTo: srv.Listener.Addr().String()}
	defer func() { http.DefaultTransport = orig }()

	stations, err := GetStations("dummy-token")
	if err != nil {
		t.Fatalf("GetStations returned error: %v", err)
	}
	if len(stations) != 1 || stations[0].StationID != 1 {
		t.Fatalf("unexpected stations result: %+v", stations)
	}
}

// TestGetStationDetails checks that GetStationDetails returns the first station in the response.
func TestGetStationDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := StationDetailsResponse{Stations: []Station{{StationID: 2, Name: "Detail", StationName: "Detail"}}}
		b, _ := json.Marshal(resp)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	orig := http.DefaultTransport
	http.DefaultTransport = &helperRoundTripper{base: orig, rewriteTo: srv.Listener.Addr().String()}
	defer func() { http.DefaultTransport = orig }()

	st, err := GetStationDetails(2, "token")
	if err != nil {
		t.Fatalf("GetStationDetails error: %v", err)
	}
	if st.StationID != 2 {
		t.Fatalf("unexpected station id: %d", st.StationID)
	}
}

// TestGetHistoricalObservationsWithProgress_Small checks that device observations are fetched and parsed for a station with a Tempest device.
func TestGetHistoricalObservationsWithProgress_Small(t *testing.T) {
	// Simulate device endpoint returning historical obs for two days
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Branch based on path: return station details or historical observations
		if strings.Contains(r.URL.Path, "/stations/") {
			resp := StationDetailsResponse{Stations: []Station{{StationID: 3, Name: "Detail", StationName: "Detail", Devices: []Device{{DeviceID: 42, DeviceType: "ST"}}}}}
			b, _ := json.Marshal(resp)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(b)
			return
		}
		if strings.Contains(r.URL.Path, "/observations/device/") {
			hr := HistoricalResponse{
				Status:    map[string]interface{}{"status": "ok"},
				StationID: 3,
				Obs: [][]interface{}{
					{float64(1000), 0.0, 0.0, 0.0, 0.0, 0.0, 1010.0, 20.0, 50.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 3.7, 60.0},
				},
			}
			b, _ := json.Marshal(hr)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(b)
			return
		}
		// default
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	orig := http.DefaultTransport
	http.DefaultTransport = &helperRoundTripper{base: orig, rewriteTo: srv.Listener.Addr().String()}
	defer func() { http.DefaultTransport = orig }()

	results, err := GetHistoricalObservationsWithProgress(3, "token", "", nil, 100)
	if err != nil {
		t.Fatalf("GetHistoricalObservationsWithProgress error: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one historical observation, got 0")
	}
}
