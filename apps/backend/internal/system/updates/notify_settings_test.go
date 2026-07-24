package updates

import "testing"

func TestDefaultNotifySettings(t *testing.T) {
	got := DefaultNotifySettings()
	if !got.Enabled {
		t.Errorf("expected Enabled=true by default")
	}
	if got.Channel != NotifyChannelBoth {
		t.Errorf("channel = %q, want %q", got.Channel, NotifyChannelBoth)
	}
}

func TestNormalizeNotifySettings_FillsDefaultChannel(t *testing.T) {
	got, err := NormalizeNotifySettings(NotifySettings{Enabled: true, Channel: ""})
	if err != nil {
		t.Fatalf("NormalizeNotifySettings: %v", err)
	}
	if got.Channel != NotifyChannelBoth {
		t.Errorf("channel = %q, want default %q", got.Channel, NotifyChannelBoth)
	}
}

func TestNormalizeNotifySettings_AcceptsKnownChannels(t *testing.T) {
	for _, channel := range []string{NotifyChannelDesktop, NotifyChannelInView, NotifyChannelBoth} {
		got, err := NormalizeNotifySettings(NotifySettings{Enabled: false, Channel: channel})
		if err != nil {
			t.Fatalf("NormalizeNotifySettings(%q): %v", channel, err)
		}
		if got.Channel != channel {
			t.Errorf("channel = %q, want %q", got.Channel, channel)
		}
		if got.Enabled {
			t.Errorf("expected Enabled=false to be preserved")
		}
	}
}

func TestNormalizeNotifySettings_RejectsUnknownChannel(t *testing.T) {
	_, err := NormalizeNotifySettings(NotifySettings{Enabled: true, Channel: "carrier-pigeon"})
	if err == nil {
		t.Fatal("expected error for unknown channel")
	}
}
