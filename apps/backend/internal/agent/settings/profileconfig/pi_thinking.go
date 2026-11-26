package profileconfig

import (
	"maps"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

// PiThinkingIntent consumes the legacy alias once per source layer without
// changing the saved map. The caller applies precedence between source layers.
func PiThinkingIntent(mode string, options map[string]string, configID string) (string, map[string]string) {
	remaining := maps.Clone(options)
	if saved := remaining[acpcompat.PiThinkingConfigID]; saved != "" {
		mode = saved
	}
	if advertised := remaining[configID]; advertised != "" {
		mode = advertised
	}
	delete(remaining, acpcompat.PiThinkingConfigID)
	delete(remaining, configID)
	return mode, remaining
}
