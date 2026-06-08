package profileconfig

import "strings"

// SanitizeConfigOptions drops reserved model/mode keys and blank entries so
// profile config options persist only auxiliary select values.
func SanitizeConfigOptions(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || key == "model" || key == "mode" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
