package types

const (
	// HostAPIPingPath is the only route a worker may probe on the host API.
	HostAPIPingPath = "/api/v1/runtime/ping"
	// HostAPIProbeTokenHeader carries the per-environment probe token.
	HostAPIProbeTokenHeader = "X-Kandev-Probe-Token"
)

// HostAPIProbeRequest asks agentctl to GET URL with the probe token. URL must
// be an http or https URL whose path is exactly HostAPIPingPath.
type HostAPIProbeRequest struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// HostAPIProbeResponse is the status code the host API answered with.
type HostAPIProbeResponse struct {
	StatusCode int `json:"status_code"`
}
