package webapp

const BootPayloadVersion = 1

// BootPayload is the JSON-safe data blob the Go server will embed in the SPA
// shell before React hydrates.
type BootPayload struct {
	Version      int                 `json:"version"`
	Route        RouteClassification `json:"route"`
	Runtime      RuntimeConfig       `json:"runtime"`
	InitialState map[string]any      `json:"initialState"`
	RouteData    map[string]any      `json:"routeData,omitempty"`
	Errors       []BootError         `json:"errors,omitempty"`
}

// RuntimeConfig contains browser-facing runtime endpoints for the SPA.
type RuntimeConfig struct {
	APIPrefix     string `json:"apiPrefix"`
	WebSocketPath string `json:"webSocketPath"`
	Debug         bool   `json:"debug,omitempty"`
}

// BootError is a serializable non-fatal boot-data error for partial hydration.
type BootError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewBootPayload(
	route RouteClassification,
	runtime RuntimeConfig,
	initialState map[string]any,
) BootPayload {
	if initialState == nil {
		initialState = map[string]any{}
	}

	return BootPayload{
		Version:      BootPayloadVersion,
		Route:        route,
		Runtime:      runtime,
		InitialState: initialState,
	}
}
