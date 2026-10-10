package models

import (
	"reflect"
	"testing"
)

func TestProjectSessionModelLabel(t *testing.T) {
	tests := []struct {
		name      string
		metadata  map[string]interface{}
		id, label string
	}{
		{name: "empty"},
		{name: "snapshot", metadata: map[string]interface{}{SessionMetaKeyACPModelState: map[string]interface{}{"current_model_id": "old", "models": []interface{}{map[string]interface{}{"model_id": "old", "name": "Old", "description": "excluded"}}, "config_options": "excluded"}}, id: "old", label: "Old"},
		{name: "runtime override", metadata: map[string]interface{}{SessionMetaKeyACPModelState: map[string]interface{}{"current_model_id": "old", "models": []interface{}{map[string]interface{}{"model_id": "override", "name": "Override"}}}, "runtime_config": map[string]interface{}{"model": "runtime"}, "runtime_config_overrides": map[string]interface{}{"model": "override"}}, id: "override", label: "Override"},
		{name: "provider-owned model", metadata: map[string]interface{}{SessionMetaKeyACPModelState: map[string]interface{}{"current_model_id": "observed", "settings_policy": "provider_restored", "models": []interface{}{map[string]interface{}{"model_id": "observed", "name": "Observed"}}}, "runtime_config_overrides": map[string]interface{}{"model": "stale-local"}}, id: "observed", label: "Observed"},
		{name: "fallback identity", metadata: map[string]interface{}{"runtime_config": map[string]interface{}{"model": "unknown"}}, id: "unknown", label: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectSessionModelLabel(tt.metadata)
			var want map[string]interface{}
			if tt.id != "" {
				want = map[string]interface{}{"current_model_id": tt.id, "models": []interface{}{map[string]interface{}{"model_id": tt.id, "name": tt.label}}}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("projection = %#v, want %#v", got, want)
			}
		})
	}
}
