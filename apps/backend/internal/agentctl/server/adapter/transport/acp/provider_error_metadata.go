package acp

import (
	"encoding/json"
	"strconv"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// mergeProviderFailureFields reads explicit optional scalar metadata only.
// ACP Data is adapter-defined: absence means unknown, never permission to parse
// a body/header or reconstruct status/code from prose. Unknown codes are kept
// for diagnostics but do not acquire provider-independent routing semantics.
func mergeProviderFailureFields(projection *streams.ProviderError, data any) {
	fields, ok := data.(map[string]any)
	if !ok {
		return
	}
	if projection.HTTPStatus == 0 {
		projection.HTTPStatus = providerHTTPStatus(fields["statusCode"])
	}
	if projection.ProviderCode == "" {
		code, _ := fields["errorCode"].(string)
		projection.ProviderCode = validProviderErrorMetadataField(code)
	}
}

func providerHTTPStatus(value any) int {
	var status int
	switch value := value.(type) {
	case int:
		status = value
	case float64:
		// JSON numbers decode as float64. Reject fractions and out-of-range
		// values before conversion rather than truncating or overflowing them.
		if value < 400 || value > 599 || value != float64(int(value)) {
			return 0
		}
		status = int(value)
	case json.Number:
		n, err := strconv.Atoi(string(value))
		if err != nil {
			return 0
		}
		status = n
	}
	if status < 400 || status > 599 {
		return 0
	}
	return status
}
