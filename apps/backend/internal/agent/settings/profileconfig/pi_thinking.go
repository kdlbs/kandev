package profileconfig

import "maps"

// PiThinkingIntent consumes the legacy alias once per source layer without
// changing the saved map. The caller applies precedence between source layers.
func PiThinkingIntent(mode string, options map[string]string, configID string) (string, map[string]string) {
	remaining := maps.Clone(options)
	if value := remaining[configID]; value != "" {
		mode = value
	}
	delete(remaining, configID)
	return mode, remaining
}
