package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/settingscatalog"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestDecodeSettingsUpdatePreservesExplicitEmptyValues(t *testing.T) {
	request, err := decodeSettingsUpdate([]byte(`{"target":{"resource_type":"agent_profile","resource_id":"profile-1"},"changes":{"auto_approve":false,"config_options":{},"env_vars":[]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if request.Changes["auto_approve"] == nil || string(request.Changes["auto_approve"]) != "false" {
		t.Fatalf("auto_approve = %s, want false", request.Changes["auto_approve"])
	}
	if string(request.Changes["config_options"]) != "{}" || string(request.Changes["env_vars"]) != "[]" {
		t.Fatalf("explicit empty values were not preserved: %#v", request.Changes)
	}
}

func TestValidateSettingsPatchRejectsUnknownAndComputedFields(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	target := settingscatalog.ResourceTarget{ResourceType: "agent_profile", ResourceID: stringPtr("profile-1")}
	if err := validateSettingsPatch(registry, target, map[string]json.RawMessage{"missing": json.RawMessage(`true`)}); err == nil {
		t.Fatal("unknown setting was accepted")
	}
	if err := validateSettingsPatch(registry, settingscatalog.ResourceTarget{ResourceType: "agent_profile_mcp", ResourceID: stringPtr("profile-1")}, map[string]json.RawMessage{"meta": json.RawMessage(`{}`)}); err == nil {
		t.Fatal("computed setting was accepted")
	}
}

func TestValidateSettingsPatchRejectsFractionalIntegers(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	target := settingscatalog.ResourceTarget{ResourceType: "user_settings"}
	if err := validateSettingsPatch(registry, target, map[string]json.RawMessage{
		"terminal_font_size": json.RawMessage(`13.5`),
	}); err == nil {
		t.Fatal("fractional integer was accepted")
	}
}

func TestDescribeSettingReturnsClosedNestedSchemas(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{settingsRegistry: registry}

	tests := []struct {
		name     string
		key      string
		target   *settingscatalog.ResourceTarget
		property string
		itemKey  string
		itemType string
		mapValue bool
	}{
		{
			name:     "keyboard shortcuts",
			key:      "user_settings.keyboard_shortcuts",
			property: "key",
			itemKey:  "key",
			itemType: "string",
			mapValue: true,
		},
		{
			name:     "workflow events",
			key:      "workflow_step.events",
			target:   &settingscatalog.ResourceTarget{ResourceType: "workflow_step", ResourceID: stringPtr("step-1"), WorkspaceID: stringPtr("workspace-1")},
			property: "on_enter",
			itemKey:  "type",
			itemType: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := map[string]any{"key": tt.key, "include_resource_schema": true}
			if tt.target != nil {
				payload["target"] = tt.target
			}
			response, err := handlers.handleDescribeSetting(context.Background(), makeWSMessage(t, ws.ActionMCPDescribeSetting, payload))
			if err != nil {
				t.Fatal(err)
			}
			if response.Type != ws.MessageTypeResponse {
				t.Fatalf("response type = %q, want response", response.Type)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Payload, &body); err != nil {
				t.Fatal(err)
			}
			schema, ok := body["schema"].(map[string]any)
			if !ok {
				t.Fatalf("schema = %#v, want object", body["schema"])
			}
			properties, ok := schema["properties"].(map[string]any)
			if tt.mapValue {
				valueSchema, valueOK := schema["additionalProperties"].(map[string]any)
				if !valueOK {
					t.Fatalf("map value schema = %#v, want object schema", schema["additionalProperties"])
				}
				valueProperties, valueOK := valueSchema["properties"].(map[string]any)
				if !valueOK {
					t.Fatalf("map value properties = %#v, want nested properties", valueSchema["properties"])
				}
				item, itemOK := valueProperties[tt.itemKey].(map[string]any)
				if !itemOK || item["type"] != tt.itemType {
					t.Fatalf("map value property %q = %#v, want type %q", tt.itemKey, valueProperties[tt.itemKey], tt.itemType)
				}
				if valueSchema["additionalProperties"] != false {
					t.Fatalf("map value additionalProperties = %#v, want false", valueSchema["additionalProperties"])
				}
				return
			}
			if !ok {
				t.Fatalf("schema properties = %#v, want nested properties", schema["properties"])
			}
			property, ok := properties[tt.property].(map[string]any)
			if !ok {
				t.Fatalf("property %q = %#v", tt.property, properties[tt.property])
			}
			items, ok := property["items"].(map[string]any)
			if !ok {
				t.Fatalf("property %q items = %#v", tt.property, property["items"])
			}
			itemProperties, ok := items["properties"].(map[string]any)
			if !ok {
				t.Fatalf("property %q item properties = %#v", tt.property, items["properties"])
			}
			item, ok := itemProperties[tt.itemKey].(map[string]any)
			if !ok || item["type"] != tt.itemType {
				t.Fatalf("property %q item %q = %#v, want type %q", tt.property, tt.itemKey, itemProperties[tt.itemKey], tt.itemType)
			}
			if !tt.mapValue && schema["additionalProperties"] != false {
				t.Fatalf("schema additionalProperties = %#v, want false", schema["additionalProperties"])
			}
		})
	}
}

func TestDescribeSettingRejectsUnsupportedOperationAndClassifiesAuthFlag(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{settingsRegistry: registry}

	unsupported, err := handlers.handleDescribeSetting(context.Background(), makeWSMessage(t, ws.ActionMCPDescribeSetting, map[string]any{
		"key": "user_settings.keyboard_shortcuts", "operation": "create",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if unsupported.Type != ws.MessageTypeError {
		t.Fatalf("unsupported operation response type = %q, want error", unsupported.Type)
	}

	authTarget := settingscatalog.ResourceTarget{ResourceType: "runtime_flag", ResourceID: stringPtr("features.auth")}
	authResponse, err := handlers.handleDescribeSetting(context.Background(), makeWSMessage(t, ws.ActionMCPDescribeSetting, map[string]any{
		"key": "runtime_flag.override", "target": authTarget,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if authResponse.Type != ws.MessageTypeResponse {
		t.Fatalf("auth response type = %q, want response", authResponse.Type)
	}
	var body map[string]any
	if err := json.Unmarshal(authResponse.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if body["support"] != string(settingscatalog.SupportException) || body["classification"] != string(settingscatalog.ClassificationException) {
		t.Fatalf("auth classification = support %v classification %v, want interactive exception", body["support"], body["classification"])
	}
	if body["writable"] != false {
		t.Fatalf("auth writable = %#v, want false", body["writable"])
	}
}

func TestDescribeSettingIncludesOperationOptionSchemas(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{settingsRegistry: registry}
	response, err := handlers.handleDescribeSetting(context.Background(), makeWSMessage(t, ws.ActionMCPDescribeSetting, map[string]any{
		"key": "storage_maintenance.enabled",
	}))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Payload, &body); err != nil {
		t.Fatal(err)
	}
	operations, ok := body["operations"].([]any)
	if !ok || len(operations) != 1 {
		t.Fatalf("operations = %#v, want one operation", body["operations"])
	}
	operation, ok := operations[0].(map[string]any)
	if !ok {
		t.Fatalf("operation = %#v, want object", operations[0])
	}
	options, ok := operation["options"].(map[string]any)
	if !ok || options["additionalProperties"] != false {
		t.Fatalf("operation options = %#v, want closed schema", operation["options"])
	}
	properties, ok := options["properties"].(map[string]any)
	if !ok {
		t.Fatalf("operation option properties = %#v, want object", options["properties"])
	}
	for _, key := range []string{"confirm_dedicated_docker", "adopt_go_cache"} {
		if _, exists := properties[key]; !exists {
			t.Fatalf("operation option %q is missing from %#v", key, properties)
		}
	}
}

func TestDescribeReadOnlySettingDefaultsToReadOperation(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{settingsRegistry: registry}
	response, err := handlers.handleDescribeSetting(context.Background(), makeWSMessage(t, ws.ActionMCPDescribeSetting, map[string]any{
		"key": "gitlab_settings.host",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Type != ws.MessageTypeResponse {
		t.Fatalf("response type = %q, want response", response.Type)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if body["operation"] != "read" {
		t.Fatalf("operation = %#v, want read", body["operation"])
	}
}

type contextualSettingsDescriptionStub struct {
	settingsResourceListStub
}

func (contextualSettingsDescriptionStub) DescribeSetting(_ context.Context, request SettingDescriptionRequest) (SettingDescription, error) {
	choice := "provider-default"
	if request.Target != nil && request.Target.ResourceID != nil {
		choice = *request.Target.ResourceID
	}
	return SettingDescription{
		Schema:  map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
		Choices: []map[string]any{{"kind": "model", "id": choice, "label": choice}},
	}, nil
}

func TestDescribeSettingResolvesTargetContextChoicesThroughHandler(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{settingsRegistry: registry, settingsOperations: contextualSettingsDescriptionStub{}}
	for _, profileID := range []string{"profile-codex", "profile-claude"} {
		response, err := handlers.handleDescribeSetting(context.Background(), makeWSMessage(t, ws.ActionMCPDescribeSetting, map[string]any{
			"key":                     "agent_profile.config_options",
			"target":                  map[string]any{"resource_type": "agent_profile", "resource_id": profileID},
			"context":                 map[string]any{"model": "selected-model"},
			"include_resource_schema": true,
		}))
		if err != nil {
			t.Fatal(err)
		}
		if response.Type != ws.MessageTypeResponse {
			t.Fatalf("response type = %q, want response", response.Type)
		}
		var body map[string]any
		if err := json.Unmarshal(response.Payload, &body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["resource_schema"]; !ok {
			t.Fatalf("resource_schema missing from %#v", body)
		}
		if _, ok := body["context"]; ok {
			t.Fatalf("request context was echoed instead of resolved: %#v", body["context"])
		}
		choices, ok := body["choices"].([]any)
		if !ok || len(choices) != 1 {
			t.Fatalf("choices = %#v, want one contextual choice", body["choices"])
		}
		choice := choices[0].(map[string]any)
		if choice["id"] != profileID {
			t.Fatalf("choice id = %#v, want %q", choice["id"], profileID)
		}
	}
}

type settingsResourceListStub struct{}

func (settingsResourceListStub) ReadSettings(context.Context, settingscatalog.ResourceTarget, []string) (any, error) {
	return map[string]any{}, nil
}

func (settingsResourceListStub) UpdateSettings(context.Context, settingscatalog.ResourceTarget, map[string]json.RawMessage, map[string]json.RawMessage) (any, error) {
	return map[string]any{}, nil
}

func (settingsResourceListStub) ListSettingsResources(context.Context, string, *string, string, int, string) (any, error) {
	return map[string]any{"resources": []any{}, "total": 0}, nil
}

func TestListSettingsResourcesHonorsRequiredOptionalAndProhibitedWorkspaceSelectors(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{settingsRegistry: registry, settingsOperations: settingsResourceListStub{}}
	workspaceID := "workspace-1"

	tests := []struct {
		name      string
		resource  string
		workspace *string
		wantType  ws.MessageType
	}{
		{name: "required workspace", resource: "jira_settings", workspace: &workspaceID, wantType: ws.MessageTypeResponse},
		{name: "optional workspace", resource: "workflow", workspace: &workspaceID, wantType: ws.MessageTypeResponse},
		{name: "prohibited workspace", resource: "workspace", workspace: &workspaceID, wantType: ws.MessageTypeError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := handlers.handleListSettingsResources(context.Background(), makeWSMessage(t, ws.ActionMCPListSettingsResources, map[string]any{
				"resource_type": tt.resource, "workspace_id": tt.workspace,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if response.Type != tt.wantType {
				t.Fatalf("response type = %q, want %q", response.Type, tt.wantType)
			}
		})
	}
}

func stringPtr(value string) *string { return &value }
