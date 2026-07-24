package updates

import "fmt"

// Update notification channels. "desktop" delivers an OS-level notification
// (native via the Tauri shell, or a browser Notification when running in a
// plain tab); "in_view" renders an in-app banner/toast that only appears
// while Kandev is open; "both" delivers both.
const (
	NotifyChannelDesktop = "desktop"
	NotifyChannelInView  = "in_view"
	NotifyChannelBoth    = "both"
)

// NotifySettings controls whether — and how — the user is notified when the
// background updates poller (see poller.go) detects a new release.
type NotifySettings struct {
	Enabled bool   `json:"enabled"`
	Channel string `json:"channel"`
}

// DefaultNotifySettings returns the value used whenever nothing has been
// saved yet (fresh installs, and existing installs on their first read after
// this feature ships): notify on both channels. Once a user saves a
// preference via NotifyStore, that choice — including disabling the feature
// — persists across restarts and future releases.
func DefaultNotifySettings() NotifySettings {
	return NotifySettings{Enabled: true, Channel: NotifyChannelBoth}
}

// NormalizeNotifySettings fills in a missing channel with the default and
// rejects anything that isn't a known channel value.
func NormalizeNotifySettings(in NotifySettings) (NotifySettings, error) {
	out := in
	if out.Channel == "" {
		out.Channel = DefaultNotifySettings().Channel
	}
	switch out.Channel {
	case NotifyChannelDesktop, NotifyChannelInView, NotifyChannelBoth:
	default:
		return NotifySettings{}, fmt.Errorf("invalid notification channel %q", in.Channel)
	}
	return out, nil
}
