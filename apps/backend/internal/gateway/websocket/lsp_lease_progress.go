package websocket

import "encoding/json"

func mergeProgressReport(previous, report, reportValue json.RawMessage) []byte {
	var prior struct {
		Value map[string]json.RawMessage `json:"value"`
	}
	if json.Unmarshal(previous, &prior) != nil || len(prior.Value) == 0 {
		return append([]byte(nil), report...)
	}
	merged := make(map[string]json.RawMessage, len(prior.Value)+4)
	for key, value := range prior.Value {
		merged[key] = append([]byte(nil), value...)
	}
	var latest map[string]json.RawMessage
	if json.Unmarshal(reportValue, &latest) != nil {
		return append([]byte(nil), report...)
	}
	for key, value := range latest {
		if key != "kind" {
			merged[key] = append([]byte(nil), value...)
		}
	}
	merged["kind"] = json.RawMessage(`"begin"`)

	var envelope map[string]json.RawMessage
	if json.Unmarshal(previous, &envelope) != nil {
		return append([]byte(nil), report...)
	}
	encodedValue, err := json.Marshal(merged)
	if err != nil {
		return append([]byte(nil), report...)
	}
	envelope["value"] = encodedValue
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return append([]byte(nil), report...)
	}
	return encoded
}
