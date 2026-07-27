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
	// InterimSettingsInterlockToken is a replayable per-boot SPA CSRF and
	// accidental-mutation interlock. It is not an authentication credential.
	InterimSettingsInterlockToken string `json:"interimSettingsInterlockToken,omitempty"`
	// Plugins lists every active, UI-bundle-declaring plugin, per
	// docs/plans/plugins/PLUGIN-API.md ("Loading model"). Populated
	// only when the plugins feature flag is on; the flag itself is not
	// carried here (see /api/v1/features) — this is purely active-bundle
	// data the frontend still gates loading on via useFeature("plugins").
	Plugins []ActivePluginPayload `json:"plugins,omitempty"`
}

// ActivePluginPayload is one entry of BootPayload.Plugins: the browser-facing
// shape the SPA's plugin host (apps/web/lib/plugins/host.ts) iterates to
// inject styles and dynamically import() each plugin's bundle.
type ActivePluginPayload struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	BundleURL string   `json:"bundleUrl"`
	StyleURLs []string `json:"styleUrls,omitempty"`
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
