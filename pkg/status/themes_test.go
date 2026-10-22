package status

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestGetTheme checks that GetTheme returns a non-nil theme for known, unknown and empty names.
func TestGetTheme(t *testing.T) {
	tests := []struct {
		name      string
		themeName string
		wantNil   bool
	}{
		{"dark ocean default", "dark-ocean", false},
		{"dark forest", "dark-forest", false},
		{"light sky", "light-sky", false},
		{"invalid theme defaults to dark-ocean", "invalid-theme", false},
		{"empty string defaults to dark-ocean", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme := GetTheme(tt.themeName)
			if tt.wantNil {
				if theme != nil {
					t.Errorf("GetTheme(%q) = %v, want nil", tt.themeName, theme)
				}
			} else {
				if theme == nil {
					t.Errorf("GetTheme(%q) = nil, want non-nil", tt.themeName)
				}
			}
		})
	}
}

// TestGetAllThemeNames checks that exactly the 12 expected theme names are listed.
func TestGetAllThemeNames(t *testing.T) {
	names := GetAllThemeNames()

	// Should have exactly 12 themes
	if len(names) != 12 {
		t.Errorf("GetAllThemeNames() returned %d themes, want 12", len(names))
	}

	// Should include known themes
	expectedThemes := []string{
		"dark-ocean", "dark-forest", "dark-sunset", "dark-twilight", "dark-matrix", "dark-cyberpunk",
		"light-sky", "light-garden", "light-autumn", "light-lavender", "light-monochrome", "light-ocean",
	}

	for _, expected := range expectedThemes {
		found := false
		for _, name := range names {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("GetAllThemeNames() missing expected theme: %s", expected)
		}
	}
}

// TestGetNextTheme checks that theme cycling advances in order and wraps around.
func TestGetNextTheme(t *testing.T) {
	tests := []struct {
		name        string
		currentName string
		wantName    string
	}{
		{"cycle from dark-ocean", "dark-ocean", "dark-forest"},
		{"cycle from dark-cyberpunk", "dark-cyberpunk", "light-sky"},
		{"cycle from light-ocean (last)", "light-ocean", "dark-ocean"},
		{"invalid current defaults to dark-ocean", "invalid", "dark-ocean"},
		{"empty current defaults to dark-ocean", "", "dark-ocean"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextTheme := GetNextTheme(tt.currentName)
			if nextTheme == nil {
				t.Errorf("GetNextTheme(%q) returned nil", tt.currentName)
				return
			}
			if nextTheme.Name != tt.wantName {
				t.Errorf("GetNextTheme(%q) = %q, want %q", tt.currentName, nextTheme.Name, tt.wantName)
			}
		})
	}
}

// TestGetLogLevelColor checks the color chosen for each log level spelling.
func TestGetLogLevelColor(t *testing.T) {
	theme := GetTheme("dark-ocean")
	if theme == nil {
		t.Fatal("GetTheme returned nil for dark-ocean")
	}

	tests := []struct {
		name  string
		level string
	}{
		{"error level", "ERROR"},
		{"warning level", "WARNING"},
		{"info level", "INFO"},
		{"debug level", "DEBUG"},
		{"unknown defaults to info", "UNKNOWN"},
		{"lowercase", "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color := theme.GetLogLevelColor(tt.level)
			if color == tcell.ColorDefault {
				t.Errorf("GetLogLevelColor(%q) returned default color", tt.level)
			}
		})
	}
}

// TestColorizeLogLine checks that log lines are wrapped in the color tag for their level, or left unchanged.
func TestColorizeLogLine(t *testing.T) {
	theme := GetTheme("dark-ocean")
	if theme == nil {
		t.Fatal("Failed to get dark-ocean theme")
	}

	tests := []struct {
		name    string
		line    string
		wantTag bool // true if we expect color tags in output
	}{
		{"error log", "2025-11-14 10:30:00 ERROR: Something went wrong", true},
		{"warn log", "2025-11-14 10:30:00 WARN: Be careful", true},
		{"info log", "2025-11-14 10:30:00 INFO: All good", true},
		{"debug log", "2025-11-14 10:30:00 DEBUG: Details here", true},
		{"plain log", "2025-11-14 10:30:00 Plain message", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := theme.ColorizeLogLine(tt.line)
			if tt.wantTag && result == tt.line {
				t.Errorf("ColorizeLogLine(%q) = %q, expected color tags", tt.line, result)
			}
			if !tt.wantTag && result != tt.line {
				t.Errorf("ColorizeLogLine(%q) = %q, expected no change", tt.line, result)
			}
		})
	}
}

// TestThemeStructure checks that the default theme has a name, description and non-default colors for every role.
func TestThemeStructure(t *testing.T) {
	theme := GetTheme("dark-ocean")
	if theme == nil {
		t.Fatal("GetTheme returned nil")
	}

	// Verify theme has required fields
	if theme.Name == "" {
		t.Error("Theme.Name is empty")
	}
	if theme.Description == "" {
		t.Error("Theme.Description is empty")
	}

	// Verify all color fields are non-default
	colorFields := []struct {
		name  string
		color tcell.Color
	}{
		{"TitleColor", theme.TitleColor},
		{"BorderColor", theme.BorderColor},
		{"LabelColor", theme.LabelColor},
		{"ValueColor", theme.ValueColor},
		{"FooterColor", theme.FooterColor},
		{"TimerColor", theme.TimerColor},
		{"ErrorColor", theme.ErrorColor},
		{"WarningColor", theme.WarningColor},
		{"InfoColor", theme.InfoColor},
		{"DebugColor", theme.DebugColor},
		{"SuccessColor", theme.SuccessColor},
		{"DangerColor", theme.DangerColor},
		{"MutedColor", theme.MutedColor},
	}

	for _, cf := range colorFields {
		if cf.color == tcell.ColorDefault {
			t.Errorf("Theme.%s is default color", cf.name)
		}
	}
}
