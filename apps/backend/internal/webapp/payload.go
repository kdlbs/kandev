package webapp

import (
	"encoding/json"
	"fmt"
	"sort"
)

const BootPayloadVersion = 2

// BootPayload is the JSON-safe data blob the Go server will embed in the SPA
// shell before React hydrates.
type BootPayload struct {
	Version      int                 `json:"version"`
	Route        RouteClassification `json:"route"`
	Runtime      RuntimeConfig       `json:"runtime"`
	InitialState map[string]any      `json:"initialState"`
	RouteData    map[string]any      `json:"routeData,omitempty"`
	Entities     *BootEntityGraph    `json:"entities,omitempty"`
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

// BootEntityGraph stores each task and session once. State and route data use
// IDs for membership; the frontend expands those IDs before store hydration.
type BootEntityGraph struct {
	Tasks    map[string]any `json:"tasks"`
	Sessions map[string]any `json:"sessions"`
}

// ActivePluginPayload is one entry of BootPayload.Plugins: the browser-facing
// shape the SPA's plugin host (apps/web/lib/plugins/host.ts) iterates to
// inject styles and dynamically import() each plugin's bundle.
type ActivePluginPayload struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	BundleURL string   `json:"bundleUrl"`
	StyleURLs []string `json:"styleUrls,omitempty"`
	// RepositoryProviderIDs is nil only for legacy manifests that omit
	// repository_providers. A non-nil empty slice is an explicit declaration
	// that must reach the browser as [] so the plugin registry can deny every
	// undeclared provider claim.
	RepositoryProviderIDs *[]string `json:"repositoryProviderIds,omitempty"`
}

// RuntimeConfig contains browser-facing runtime endpoints for the SPA.
type RuntimeConfig struct {
	APIPrefix                         string   `json:"apiPrefix"`
	WebSocketPath                     string   `json:"webSocketPath"`
	BootID                            string   `json:"bootId,omitempty"`
	LSPAutoInstallPreferenceLanguages []string `json:"lspAutoInstallPreferenceLanguages,omitempty"`
	Debug                             bool     `json:"debug,omitempty"`
	// NonProduction marks a dev or e2e build. Distinct from Debug (which the SPA
	// uses for verbose logging): this gates QA-only UI such as the pseudo-locale
	// option, which the e2e harness needs even though it serves a PRODUCTION
	// frontend bundle — so `import.meta.env.PROD` cannot answer this question.
	NonProduction bool `json:"nonProduction,omitempty"`
	// Locale is the active UI locale (BCP-47-ish tag) the SPA should activate
	// before first paint. Sourced from the kandev_locale cookie; defaults to
	// "en". Also drives the shell's <html lang> so first paint matches.
	Locale string `json:"locale,omitempty"`
	// TitlePrefix distinguishes Kandev instances in adjacent browser tabs. It
	// carries the operator-configured prefix (KANDEV_WEB_TITLE_PREFIX) with
	// surrounding whitespace trimmed, not the composed title: the shell
	// rewrites <title> server-side for first paint, and the SPA composes the
	// same "<prefix> Kandev" for the
	// /api/v1/app-state boot path, which never renders through the shell.
	TitlePrefix string `json:"titlePrefix,omitempty"`
	// NativeFolderPickerAvailable is true only when the desktop shell launched
	// this backend and can service the narrow native folder-picker command.
	// Browsers must continue to use the HTTP directory picker.
	NativeFolderPickerAvailable bool `json:"nativeFolderPickerAvailable,omitempty"`
	// DesktopRuntime identifies the launch policy selected by the backend
	// process marker. It is separate from the native picker capability so an
	// ordinary browser connected to a desktop backend keeps desktop discovery
	// policy while using the HTTP picker.
	DesktopRuntime bool `json:"desktopRuntime,omitempty"`
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

// NormalizeBootPayloadGraph converts repeated task/session records into one
// entity table with ID membership. A JSON round trip gives the normalizer one
// stable representation for the many typed DTO slices used by boot builders.
func NormalizeBootPayloadGraph(payload *BootPayload) error {
	if payload == nil {
		return fmt.Errorf("boot payload is nil")
	}
	state, err := bootJSONMap(payload.InitialState)
	if err != nil {
		return fmt.Errorf("encode boot initial state: %w", err)
	}
	routeData, err := bootJSONMap(payload.RouteData)
	if err != nil {
		return fmt.Errorf("encode boot route data: %w", err)
	}
	entities := &BootEntityGraph{
		Tasks:    map[string]any{},
		Sessions: map[string]any{},
	}
	normalizeBootNode(state, "", entities)
	normalizeBootNode(routeData, "", entities)
	payload.Version = BootPayloadVersion
	payload.InitialState = state
	if len(routeData) == 0 {
		payload.RouteData = nil
	} else {
		payload.RouteData = routeData
	}
	payload.Entities = entities
	return nil
}

func bootJSONMap(value map[string]any) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if result == nil {
		result = map[string]any{}
	}
	return result, nil
}

func normalizeBootNode(value any, parent string, entities *BootEntityGraph) {
	switch node := value.(type) {
	case map[string]any:
		if parent == "taskSessions" {
			normalizeBootSessionItems(node, entities)
		}
		if parent == "taskSessionsByTask" {
			normalizeBootSessionsByTask(node, entities)
		}
		keys := make([]string, 0, len(node))
		for key := range node {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child, exists := node[key]
			if !exists {
				continue
			}
			if normalizeBootField(node, key, child, entities) {
				continue
			}
			normalizeBootNode(child, key, entities)
		}
	case []any:
		for _, child := range node {
			normalizeBootNode(child, parent, entities)
		}
	}
}

func normalizeBootTaskList(value any, entities *BootEntityGraph) ([]any, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	ids := make([]any, 0, len(items))
	for _, item := range items {
		task, ok := item.(map[string]any)
		if !ok || !isBootTaskEntity(task) {
			return nil, false
		}
		id := addBootEntity(entities.Tasks, task)
		if id == "" {
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

func normalizeBootSessionItems(node map[string]any, entities *BootEntityGraph) {
	items, ok := node["items"].(map[string]any)
	if !ok {
		return
	}
	ids := make([]any, 0, len(items))
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		session, ok := items[key].(map[string]any)
		if !ok || !isBootSessionEntity(session) {
			return
		}
		id := addBootEntity(entities.Sessions, session)
		if id == "" {
			return
		}
		ids = append(ids, id)
	}
	delete(node, "items")
	node["sessionIds"] = ids
}

func normalizeBootSessionsByTask(node map[string]any, entities *BootEntityGraph) {
	items, ok := node["itemsByTaskId"].(map[string]any)
	if !ok {
		return
	}
	normalized := make(map[string]any, len(items))
	for taskID, value := range items {
		sessions, ok := value.([]any)
		if !ok {
			return
		}
		ids := make([]any, 0, len(sessions))
		for _, item := range sessions {
			session, ok := item.(map[string]any)
			if !ok || !isBootSessionEntity(session) {
				return
			}
			id := addBootEntity(entities.Sessions, session)
			if id == "" {
				return
			}
			ids = append(ids, id)
		}
		normalized[taskID] = ids
	}
	delete(node, "itemsByTaskId")
	node["sessionIdsByTask"] = normalized
}

func isBootTaskEntity(value map[string]any) bool {
	if bootString(value["id"]) == "" || bootString(value["title"]) == "" {
		return false
	}
	return (bootString(value["workflow_id"]) != "" && bootString(value["workspace_id"]) != "") ||
		(bootString(value["workflowId"]) != "" && bootString(value["workflowStepId"]) != "")
}

func isBootSessionEntity(value map[string]any) bool {
	return bootString(value["id"]) != "" && bootString(value["task_id"]) != "" && bootString(value["state"]) != ""
}

func addBootEntity(entities map[string]any, value map[string]any) string {
	id := bootString(value["id"])
	if id == "" {
		return ""
	}
	if existing, ok := entities[id].(map[string]any); ok {
		entities[id] = mergeBootEntity(existing, value)
	} else {
		entities[id] = value
	}
	return id
}

func mergeBootEntity(existing, incoming map[string]any) map[string]any {
	merged := make(map[string]any, len(existing)+len(incoming))
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range incoming {
		if old, ok := merged[key].(map[string]any); ok {
			if next, ok := value.(map[string]any); ok {
				merged[key] = mergeBootEntity(old, next)
				continue
			}
		}
		merged[key] = value
	}
	return merged
}

func bootString(value any) string {
	result, _ := value.(string)
	return result
}

func normalizeBootField(node map[string]any, key string, child any, entities *BootEntityGraph) bool {
	if key == "tasks" {
		if ids, ok := normalizeBootTaskList(child, entities); ok {
			delete(node, key)
			node["taskIds"] = ids
			return true
		}
	}
	if key != "task" {
		return false
	}
	task, ok := child.(map[string]any)
	if !ok || !isBootTaskEntity(task) {
		return false
	}
	id := addBootEntity(entities.Tasks, task)
	if id == "" {
		return false
	}
	delete(node, key)
	node["taskId"] = id
	return true
}
