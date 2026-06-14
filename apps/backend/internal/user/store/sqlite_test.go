package store

import (
	"testing"
	"time"
)

type settingsScanner struct {
	raw string
}

func (s settingsScanner) Scan(dest ...any) error {
	*(dest[0].(*string)) = s.raw
	*(dest[1].(*time.Time)) = time.Time{}
	return nil
}

func TestScanUserSettingsChangesPanelLayoutDefault(t *testing.T) {
	t.Run("empty settings default to tree", func(t *testing.T) {
		settings, err := scanUserSettings(settingsScanner{raw: "{}"}, DefaultUserID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ChangesPanelLayout != "tree" {
			t.Fatalf("expected ChangesPanelLayout=tree, got %q", settings.ChangesPanelLayout)
		}
	})

	t.Run("missing layout defaults to tree", func(t *testing.T) {
		settings, err := scanUserSettings(settingsScanner{raw: `{"chat_submit_key":"cmd_enter"}`}, DefaultUserID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ChangesPanelLayout != "tree" {
			t.Fatalf("expected ChangesPanelLayout=tree, got %q", settings.ChangesPanelLayout)
		}
	})

	t.Run("explicit flat is preserved", func(t *testing.T) {
		settings, err := scanUserSettings(settingsScanner{raw: `{"changes_panel_layout":"flat"}`}, DefaultUserID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ChangesPanelLayout != "flat" {
			t.Fatalf("expected ChangesPanelLayout=flat, got %q", settings.ChangesPanelLayout)
		}
	})
}

func TestScanUserSettingsSystemMetricsDisplayDefault(t *testing.T) {
	settings, err := scanUserSettings(settingsScanner{raw: "{}"}, DefaultUserID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.SystemMetricsDisplay.ShowInTopbar {
		t.Fatal("system metrics display should default to disabled")
	}

	settings, err = scanUserSettings(settingsScanner{raw: `{"system_metrics_display":{"show_in_topbar":true}}`}, DefaultUserID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !settings.SystemMetricsDisplay.ShowInTopbar {
		t.Fatal("expected stored system metrics display preference")
	}
}
