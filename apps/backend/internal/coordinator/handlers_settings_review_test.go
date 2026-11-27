package coordinator

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestSettingsSavePreservesAutonomyAndPauseDTO(t *testing.T) {
	h, _, c := settingsHandlers(t)
	h.service.phase3 = true
	h.service.phase31 = true
	rec := runHandler(h.httpPutSettings, http.MethodPut, "/x", `{"watches":{"scope":"selected","workflow_ids":["wf-a"]}}`, workspaceParams(c.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", rec.Code, rec.Body.String())
	}
	var raw map[string]json.RawMessage
	decodeBody(t, rec, &raw)
	for _, key := range []string{"autonomy_enabled", "cost_ceiling_usd", "paused", "paused_at", "paused_by"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("settings-save DTO omits %s: %s", key, rec.Body.String())
		}
	}
}
