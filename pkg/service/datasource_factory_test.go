package service

import (
	"testing"
	"time"

	"tempest-homekit-go/pkg/config"
	"tempest-homekit-go/pkg/weather"
)

// fakeUDPListener is a dummy type to satisfy weather.UDPListener interface used in
// CreateDataSource tests. It only implements GetLatestObservation, so it does not
// actually satisfy the interface and is unused apart from the blank reference
// below.
type fakeUDPListener struct{}

// GetLatestObservation always returns nil.
func (f fakeUDPListener) GetLatestObservation() *weather.Observation { return nil }

// Mark fakeUDPListener as referenced to avoid unused warnings in some tooling.
var _ = fakeUDPListener{}

// TestCreateDataSource_UDPRequiresListener checks that UDP mode fails without a listener.
func TestCreateDataSource_UDPRequiresListener(t *testing.T) {
	cfg := &config.Config{UDPStream: true}
	_, err := CreateDataSource(cfg, nil, nil, nil)
	if err == nil {
		t.Fatalf("expected error when UDPStream enabled but listener is nil")
	}
}

// TestCreateDataSource_CustomStationURL checks that a custom station URL yields a non-nil data source.
func TestCreateDataSource_CustomStationURL(t *testing.T) {
	cfg := &config.Config{StationURL: "https://example.com/foo", DisableInternet: false}
	ds, err := CreateDataSource(cfg, &weather.Station{StationID: 42, StationName: "S"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error creating API data source: %v", err)
	}
	if ds == nil {
		t.Fatalf("expected non-nil datasource for custom StationURL")
	}
}

// TestCreateDataSource_GeneratedWeatherDefaults checks that generated-weather mode yields a non-nil data source.
func TestCreateDataSource_GeneratedWeatherDefaults(t *testing.T) {
	cfg := &config.Config{UseGeneratedWeather: true}
	ds, err := CreateDataSource(cfg, &weather.Station{StationID: 1, StationName: "S"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error creating generated weather data source: %v", err)
	}
	if ds == nil {
		t.Fatalf("expected non-nil datasource for generated weather")
	}
}

// mockUDPListener implements the weather.UDPListener interface for testing. Tests
// set its unexported fields directly to control what GetStats, GetObservations
// and GetLatestObservation report; Start and Stop only record that they were
// called.
type mockUDPListener struct {
	started      bool
	stopped      bool
	observations []weather.Observation
	obsChan      chan weather.Observation
	stopChan     chan struct{}
	packetCount  int64
	lastPacket   time.Time
	stationIP    string
	serialNumber string
}

// newMockUDPListener returns a mockUDPListener with a buffered observation channel and a fixed station IP and serial number.
func newMockUDPListener() *mockUDPListener {
	return &mockUDPListener{
		observations: []weather.Observation{},
		obsChan:      make(chan weather.Observation, 10),
		stopChan:     make(chan struct{}),
		stationIP:    "192.168.1.100",
		serialNumber: "ST-00000001",
	}
}

// Start records that Start was called and returns nil.
func (m *mockUDPListener) Start() error {
	m.started = true
	return nil
}

// Stop records that Stop was called, closes stopChan and returns nil. Calling it twice panics.
func (m *mockUDPListener) Stop() error {
	m.stopped = true
	if m.stopChan != nil {
		close(m.stopChan)
	}
	return nil
}

// GetLatestObservation returns a pointer to the last element of observations, or nil if there are none.
func (m *mockUDPListener) GetLatestObservation() *weather.Observation {
	if len(m.observations) == 0 {
		return nil
	}
	return &m.observations[len(m.observations)-1]
}

// GetStats returns the configured packet count, last packet time, station IP and serial number.
func (m *mockUDPListener) GetStats() (packetCount int64, lastPacket time.Time, stationIP, serialNumber string) {
	return m.packetCount, m.lastPacket, m.stationIP, m.serialNumber
}

// GetObservations returns a copy of observations.
func (m *mockUDPListener) GetObservations() []weather.Observation {
	result := make([]weather.Observation, len(m.observations))
	copy(result, m.observations)
	return result
}

// ObservationChannel returns obsChan.
func (m *mockUDPListener) ObservationChannel() <-chan weather.Observation {
	return m.obsChan
}

// IsReceivingData reports whether any observations have been set.
func (m *mockUDPListener) IsReceivingData() bool {
	return len(m.observations) > 0
}

// GetDeviceStatus always returns nil.
func (m *mockUDPListener) GetDeviceStatus() interface{} {
	return nil
}

// GetHubStatus always returns nil.
func (m *mockUDPListener) GetHubStatus() interface{} {
	return nil
}

// mockStation creates a test station with ID 12345 named "Test Station".
func mockStation() *weather.Station {
	return &weather.Station{
		StationID:   12345,
		Name:        "Test Station",
		StationName: "Test Station",
	}
}

// TestCreateDataSource_UDPMode checks that UDP mode yields a UDP data source whose status reports the UDP type.
func TestCreateDataSource_UDPMode(t *testing.T) {
	cfg := &config.Config{
		UDPStream:       true,
		DisableInternet: true,
		StationName:     "Test Station",
		Token:           "test-token",
	}

	station := mockStation()
	udpListener := newMockUDPListener()

	dataSource, err := CreateDataSource(cfg, station, udpListener, nil)
	if err != nil {
		t.Fatalf("Failed to create UDP data source: %v", err)
	}

	// Verify it's the correct type
	if dataSource.GetType() != weather.DataSourceUDP {
		t.Errorf("Expected UDP data source, got %s", dataSource.GetType())
	}

	// Verify status
	status := dataSource.GetStatus()
	if status.Type != weather.DataSourceUDP {
		t.Errorf("Expected status type UDP, got %s", status.Type)
	}
	// UDP data source gets station info from broadcasts, not initial config
	if !status.Active {
		t.Log("Note: Status not yet active (no data received, expected in test)")
	}
}

// TestCreateDataSource_CustomURL checks that a custom URL yields a custom-URL data source whose status carries that URL.
func TestCreateDataSource_CustomURL(t *testing.T) {
	cfg := &config.Config{
		StationURL:  "http://localhost:8080/weather",
		StationName: "Test Station",
		Token:       "test-token",
	}

	station := mockStation()

	dataSource, err := CreateDataSource(cfg, station, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create custom URL data source: %v", err)
	}

	// Custom URL uses API data source type
	if dataSource.GetType() != weather.DataSourceCustomURL {
		t.Errorf("Expected CustomURL data source, got %s", dataSource.GetType())
	}

	// Verify status
	status := dataSource.GetStatus()
	if status.Type != weather.DataSourceCustomURL {
		t.Errorf("Expected status type CustomURL, got %s", status.Type)
	}
	if status.CustomURL != "http://localhost:8080/weather" {
		t.Errorf("Expected URL in status, got %s", status.CustomURL)
	}
}

// TestCreateDataSource_GeneratedWeather checks that generated-weather mode yields a generated data source and status type.
func TestCreateDataSource_GeneratedWeather(t *testing.T) {
	cfg := &config.Config{
		UseGeneratedWeather: true,
		StationName:         "Test Station",
		Token:               "test-token",
	}

	station := mockStation()

	dataSource, err := CreateDataSource(cfg, station, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create generated weather data source: %v", err)
	}

	// Generated weather uses API data source with localhost URL
	if dataSource.GetType() != weather.DataSourceGenerated {
		t.Errorf("Expected Generated data source, got %s", dataSource.GetType())
	}

	// Verify status
	status := dataSource.GetStatus()
	if status.Type != weather.DataSourceGenerated {
		t.Errorf("Expected status type Generated, got %s", status.Type)
	}
}

// TestCreateDataSource_RealAPI checks that with no mode flags the WeatherFlow API data source is chosen and reports the station name.
func TestCreateDataSource_RealAPI(t *testing.T) {
	cfg := &config.Config{
		StationName: "Test Station",
		Token:       "test-token",
	}

	station := mockStation()

	dataSource, err := CreateDataSource(cfg, station, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create API data source: %v", err)
	}

	// Should default to API
	if dataSource.GetType() != weather.DataSourceAPI {
		t.Errorf("Expected API data source, got %s", dataSource.GetType())
	}

	// Verify status
	status := dataSource.GetStatus()
	if status.Type != weather.DataSourceAPI {
		t.Errorf("Expected status type API, got %s", status.Type)
	}
	if status.StationName != station.Name {
		t.Errorf("Expected station name %s, got %s", station.Name, status.StationName)
	}
}

// TestCreateDataSource_Priority checks that UDP mode wins over a custom URL and generated weather.
func TestCreateDataSource_Priority(t *testing.T) {
	// Test that UDP takes priority over everything
	cfg := &config.Config{
		UDPStream:           true,
		UseGeneratedWeather: true,
		StationURL:          "http://custom.url",
		StationName:         "Test Station",
		Token:               "test-token",
	}

	station := mockStation()
	udpListener := newMockUDPListener()

	dataSource, err := CreateDataSource(cfg, station, udpListener, nil)
	if err != nil {
		t.Fatalf("Failed to create data source: %v", err)
	}

	if dataSource.GetType() != weather.DataSourceUDP {
		t.Errorf("Expected UDP to take priority, got %s", dataSource.GetType())
	}
}

// TestCreateDataSource_CustomURLPriorityOverGenerated checks that a custom URL wins over generated weather.
func TestCreateDataSource_CustomURLPriorityOverGenerated(t *testing.T) {
	// Test that Custom URL takes priority over Generated
	cfg := &config.Config{
		StationURL:          "http://custom.url",
		UseGeneratedWeather: true,
		StationName:         "Test Station",
		Token:               "test-token",
	}

	station := mockStation()

	dataSource, err := CreateDataSource(cfg, station, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create data source: %v", err)
	}

	if dataSource.GetType() != weather.DataSourceCustomURL {
		t.Errorf("Expected CustomURL to take priority over Generated, got %s", dataSource.GetType())
	}
}

// TestCreateDataSource_UDPWithoutListener checks that UDP mode fails when the listener is nil.
func TestCreateDataSource_UDPWithoutListener(t *testing.T) {
	cfg := &config.Config{
		UDPStream:   true,
		StationName: "Test Station",
		Token:       "test-token",
	}

	station := mockStation()

	_, err := CreateDataSource(cfg, station, nil, nil)
	if err == nil {
		t.Error("Expected error when UDP mode enabled but no listener provided")
	}
}

// TestCreateDataSource_InvalidUDPListenerType checks that UDP mode fails when the listener does not implement weather.UDPListener.
func TestCreateDataSource_InvalidUDPListenerType(t *testing.T) {
	cfg := &config.Config{
		UDPStream:   true,
		StationName: "Test Station",
		Token:       "test-token",
	}

	station := mockStation()
	invalidListener := "not a listener" // Wrong type

	_, err := CreateDataSource(cfg, station, invalidListener, nil)
	if err == nil {
		t.Error("Expected error when invalid listener type provided")
	}
}

// TestCreateDataSource_NilStation checks the error returned when the default API mode has no station.
func TestCreateDataSource_NilStation(t *testing.T) {
	cfg := &config.Config{
		StationName: "Test Station",
		Token:       "test-token",
	}

	_, err := CreateDataSource(cfg, nil, nil, nil)
	if err == nil {
		t.Fatal("Expected error when creating API data source with nil station")
	}

	// API data source requires station info
	if err.Error() != "station required for API data source" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

// TestDataSourceStatus_Structure checks that an API data source status has a type and station name.
func TestDataSourceStatus_Structure(t *testing.T) {
	cfg := &config.Config{
		StationName: "Test Station",
		Token:       "test-token",
	}

	station := mockStation()

	dataSource, err := CreateDataSource(cfg, station, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create data source: %v", err)
	}

	status := dataSource.GetStatus()

	// Verify all required fields are present
	if status.Type == "" {
		t.Error("Status type is empty")
	}
	if status.StationName == "" {
		t.Error("Status station name is empty")
	}
	if status.LastUpdate.IsZero() {
		t.Log("Note: LastUpdate is zero (no data fetched yet, expected in test)")
	}
}

// TestDataSourceInterfaces checks that an API data source can be queried, started
// and stopped without error. Starting it polls the real WeatherFlow API with a
// fake token.
func TestDataSourceInterfaces(t *testing.T) {
	// Verify all data source types implement the interface correctly
	cfg := &config.Config{
		StationName: "Test Station",
		Token:       "test-token",
	}

	station := mockStation()

	dataSource, err := CreateDataSource(cfg, station, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create data source: %v", err)
	}

	// Test interface methods don't panic
	_ = dataSource.GetType()
	_ = dataSource.GetStatus()
	_ = dataSource.GetForecast() // May return nil initially

	// Start and stop should work
	obsChan, err := dataSource.Start()
	if err != nil {
		t.Fatalf("Failed to start data source: %v", err)
	}

	if obsChan == nil {
		t.Error("Expected non-nil observation channel")
	}

	// Give it a moment
	time.Sleep(100 * time.Millisecond)

	err = dataSource.Stop()
	if err != nil {
		t.Errorf("Failed to stop data source: %v", err)
	}
}

// TestCreateDataSource_StationURLConstruction checks that generated-weather mode
// with a custom web port and generator path yields a generated data source whose
// status has no custom URL. (It was described as checking that the generated
// StationURL is built the way LoadConfig builds it, but no URL is inspected.)
func TestCreateDataSource_StationURLConstruction(t *testing.T) {
	cfg := &config.Config{
		UseGeneratedWeather:  true,
		WebPort:              "12345",
		GeneratedWeatherPath: "/api/custom-generate",
		StationName:          "Test Station",
		Token:                "test-token",
	}

	station := mockStation()

	ds, err := CreateDataSource(cfg, station, nil, nil)
	if err != nil {
		t.Fatalf("CreateDataSource failed: %v", err)
	}

	// For generated weather, the data source type should be Generated
	if ds.GetType() != weather.DataSourceGenerated {
		t.Fatalf("expected Generated data source type, got %s", ds.GetType())
	}

	status := ds.GetStatus()

	// For generated data sources, CustomURL is not applicable
	if status.CustomURL != "" {
		t.Fatalf("expected empty CustomURL for generated data source, got %s", status.CustomURL)
	}
}

// TestCreateDataSource_GeneratedDefaults checks generated-weather mode with empty web port and generator path.
func TestCreateDataSource_GeneratedDefaults(t *testing.T) {
	// When WebPort and GeneratedWeatherPath are empty, defaults should be used
	cfg := &config.Config{
		UseGeneratedWeather:  true,
		WebPort:              "",
		GeneratedWeatherPath: "",
		StationName:          "Test Station",
		Token:                "test-token",
	}

	station := mockStation()

	ds, err := CreateDataSource(cfg, station, nil, nil)
	if err != nil {
		t.Fatalf("CreateDataSource failed: %v", err)
	}

	// For generated weather, the data source type should be Generated
	if ds.GetType() != weather.DataSourceGenerated {
		t.Fatalf("expected Generated data source type, got %s", ds.GetType())
	}

	status := ds.GetStatus()

	// For generated data sources, CustomURL is not applicable
	if status.CustomURL != "" {
		t.Fatalf("expected empty CustomURL for generated data source, got %s", status.CustomURL)
	}
}

// TestCreateDataSource_CustomURLWithNilStation checks that a custom URL works without a station pointer.
func TestCreateDataSource_CustomURLWithNilStation(t *testing.T) {
	// Ensure that providing StationURL does not require a station pointer
	cfg := &config.Config{
		StationURL: "http://example.local/weather",
		Token:      "test-token",
	}

	ds, err := CreateDataSource(cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateDataSource failed when station nil with custom URL: %v", err)
	}

	if ds.GetType() != weather.DataSourceCustomURL {
		t.Fatalf("expected CustomURL type, got %s", ds.GetType())
	}

	status := ds.GetStatus()
	if status.CustomURL != "http://example.local/weather" {
		t.Fatalf("expected custom URL in status, got %s", status.CustomURL)
	}
}
