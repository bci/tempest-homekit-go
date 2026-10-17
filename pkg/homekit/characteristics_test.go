package homekit

import "testing"

// TestCustomCharacteristicsConstructors checks that each custom characteristic constructor returns a non-nil value.
func TestCustomCharacteristicsConstructors(t *testing.T) {
	ws := NewWindSpeedCharacteristic()
	if ws == nil {
		t.Fatal("NewWindSpeedCharacteristic returned nil")
	}

	wd := NewWindDirectionCharacteristic()
	if wd == nil {
		t.Fatal("NewWindDirectionCharacteristic returned nil")
	}

	ra := NewRainAccumulationCharacteristic()
	if ra == nil {
		t.Fatal("NewRainAccumulationCharacteristic returned nil")
	}

	uv := NewUVIndexCharacteristic()
	if uv == nil {
		t.Fatal("NewUVIndexCharacteristic returned nil")
	}

	lc := NewLightningCountCharacteristic()
	if lc == nil {
		t.Fatal("NewLightningCountCharacteristic returned nil")
	}

	pd := NewPrecipitationTypeCharacteristic()
	if pd == nil {
		t.Fatal("NewPrecipitationTypeCharacteristic returned nil")
	}

	pc := NewPressureCharacteristic()
	if pc == nil {
		t.Fatal("NewPressureCharacteristic returned nil")
	}
}
