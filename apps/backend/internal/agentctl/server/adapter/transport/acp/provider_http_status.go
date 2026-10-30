package acp

// Only explicit HTTP metadata is admitted. JSON-RPC codes and numbers in an
// error message are not HTTP status evidence.
func providerHTTPStatusFromData(data any) int {
	fields, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	for _, key := range []string{"statusCode", "httpStatus"} {
		var status int
		switch value := fields[key].(type) {
		case float64:
			if value < 100 || value > 599 || value != float64(int(value)) {
				continue
			}
			status = int(value)
		case int:
			status = value
		}
		if status >= 100 && status <= 599 {
			return status
		}
	}
	return 0
}
