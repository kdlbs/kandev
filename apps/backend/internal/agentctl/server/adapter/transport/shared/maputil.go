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

// GetBool extracts a bool value from a map, returning false if not found or wrong type.
func GetBool(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

// GetMap extracts a nested map from a map, returning nil if not found or wrong type.
func GetMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return nil
}

// GetSlice extracts a slice from a map, returning nil if not found or wrong type.
func GetSlice(m map[string]any, key string) []any {
	if v, ok := m[key].([]any); ok {
		return v
	}
	return nil
}
