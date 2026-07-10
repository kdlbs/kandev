package shared

// GetString extracts a string value from a map, returning empty string if not found or wrong type.
func GetString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// GetInt extracts an int value from a map, returning 0 if not found or wrong type.
// Handles JSON numbers which are decoded as float64.
func GetInt(m map[string]any, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}
