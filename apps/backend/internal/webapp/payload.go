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
	// docs/plans/plugins/PLUGIN-API.md ("Loading model"). Empty when the
	// plugin service failed to initialize or nothing active declares a
	// bundle; the frontend boots whatever it finds here unconditionally.
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
	// NonProduction marks a dev or e2e build. Distinct from Debug (which the SPA
	// uses for verbose logging): this gates QA-only UI such as the pseudo-locale
	// option, which the e2e harness needs even though it serves a PRODUCTION
	// frontend bundle — so `import.meta.env.PROD` cannot answer this question.
	NonProduction bool `json:"nonProduction,omitempty"`
	// Locale is the active UI locale (BCP-47-ish tag) the SPA should activate
	// before first paint. Sourced from the kandev_locale cookie; defaults to
	// "en". Also drives the shell's <html lang> so first paint matches.
	Locale string `json:"locale,omitempty"`
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
