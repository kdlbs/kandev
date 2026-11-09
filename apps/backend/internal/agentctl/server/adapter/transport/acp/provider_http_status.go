package acp

// Only explicit HTTP metadata is admitted. JSON-RPC codes and numbers in an
// error message are not HTTP status evidence.
func providerHTTPStatusFromData(data any) int {
	fields, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	for _, key := range []string{"statusCode", "httpStatus"} {
		if status := providerHTTPStatus(fields[key]); status != 0 {
			return status
		}
	}
	return 0
}
