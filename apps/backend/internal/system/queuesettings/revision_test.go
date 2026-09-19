package queuesettings

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.11
func TestAutoMergeRevisionAdvancesOnlyWhenBooleanChangesAndStaysInternal(t *testing.T) {
	raw := &fakeRawStore{}
	target := &fakeTarget{max: DefaultMaxPerSession, mergeEnabled: true, autoMergeEnabled: true}
	service := NewService(NewStore(raw), target, func() Environment { return Environment{} }, testLogger(t))
	ctx := context.Background()

	if _, err := service.Update(ctx, SettingsPatch{AutoMergeEnabled: new(false)}); err != nil {
		t.Fatalf("disable Auto-merge: %v", err)
	}
	assertStoredAutoMergeRevision(t, raw.raw, 1)
	if _, err := service.Update(ctx, SettingsPatch{MaxPerSession: new(4)}); err != nil {
		t.Fatalf("update capacity: %v", err)
	}
	assertStoredAutoMergeRevision(t, raw.raw, 1)
	response, err := service.Update(ctx, SettingsPatch{AutoMergeEnabled: new(true)})
	if err != nil {
		t.Fatalf("enable Auto-merge: %v", err)
	}
	assertStoredAutoMergeRevision(t, raw.raw, 2)

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(string(encoded), "auto_merge_revision") {
		t.Fatalf("public response leaked internal revision: %s", encoded)
	}
}

func assertStoredAutoMergeRevision(t *testing.T, raw []byte, want float64) {
	t.Helper()
	var stored map[string]interface{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("decode stored settings: %v", err)
	}
	if got := stored["auto_merge_revision"]; got != want {
		t.Fatalf("stored auto_merge_revision = %v, want %.0f", got, want)
	}
}
