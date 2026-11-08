package sessioncapacity

import (
	"context"
	"encoding/json"
	"testing"
)

type gradePolicyTarget struct {
	effective   Effective
	legacyCalls int
}

func (t *gradePolicyTarget) SetSessionCapacity(int)           { t.legacyCalls++ }
func (t *gradePolicyTarget) SetExecutionCapacity(e Effective) { t.effective = e }

func TestGradePolicySettingsUseExistingStoreAndLiveTarget(t *testing.T) {
	ctx := context.Background()
	raw := &memoryRawStore{}
	target := &gradePolicyTarget{}
	service := NewService(NewStore(raw), target, Environment{}, testLogger(t))
	var patch SettingsPatch
	if err := json.Unmarshal([]byte(`{"enabled":true,"max_sessions":3,"policy":"all","grade_limits":{"A":1,"B":0}}`), &patch); err != nil {
		t.Fatalf("grade policy patch rejected: %v", err)
	}
	response, err := service.Update(ctx, patch)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var fields struct {
		Settings struct {
			Policy      string
			GradeLimits map[string]int `json:"grade_limits"`
		}
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields.Settings.Policy != "all" || fields.Settings.GradeLimits["A"] != 1 {
		t.Fatalf("settings=%s", data)
	}
	if _, present := fields.Settings.GradeLimits["B"]; !present {
		t.Fatal("zero grade limit was lost")
	}
	if target.legacyCalls != 0 {
		t.Fatal("live target received only total capacity")
	}
	if err := json.Unmarshal([]byte(`{"max_sessions":4}`), &patch); err != nil {
		t.Fatal(err)
	}
	next, err := service.Update(ctx, patch)
	if err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields.Settings.Policy != "all" || fields.Settings.GradeLimits["A"] != 1 {
		t.Fatal("omitted patch discarded grade policy")
	}
	loaded, err := NewStore(raw).Load(ctx)
	if err != nil || loaded == nil || loaded.Policy != PolicyAll || loaded.GradeLimits["A"] != 1 || loaded.MaxSessions != 4 {
		t.Fatalf("saved settings=%+v err=%v", loaded, err)
	}
}

func TestGradePolicyNullAndInvalidFieldsAreRejected(t *testing.T) {
	for _, raw := range []string{`{"policy":null}`, `{"grade_limits":null}`, `{"grade_limits":{"A":null}}`, `{"grade_limits":{"A":1.5}}`} {
		var patch SettingsPatch
		if err := json.Unmarshal([]byte(raw), &patch); err == nil {
			t.Fatalf("invalid patch accepted: %s", raw)
		}
	}
}

func TestGradePolicyDisabledAndEnvironmentKeepLegacyEnforcement(t *testing.T) {
	saved := Settings{Enabled: false, MaxSessions: 4, Policy: PolicyAll, GradeLimits: map[string]int{"A": 1}}
	disabled, err := Resolve(&saved, Environment{})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Effective.Enabled || disabled.Effective.MaxSessions != 0 || disabled.Effective.Policy != PolicyAutomatic || len(disabled.Effective.GradeLimits) != 0 {
		t.Fatalf("disabled effective=%+v", disabled.Effective)
	}
	saved.Enabled = true
	overridden, err := Resolve(&saved, Environment{Value: "3", Present: true})
	if err != nil {
		t.Fatal(err)
	}
	if overridden.Effective.Policy != PolicyAutomatic || overridden.Effective.MaxSessions != 3 || len(overridden.Effective.GradeLimits) != 0 || !overridden.Effective.Locked {
		t.Fatalf("environment effective=%+v", overridden.Effective)
	}
	if saved.Policy != PolicyAll || saved.GradeLimits["A"] != 1 {
		t.Fatal("environment erased remembered grade settings")
	}
}

func TestGradePolicyRequiresCapableLiveTargetBeforeSaving(t *testing.T) {
	raw := &memoryRawStore{}
	service := NewService(NewStore(raw), &fakeTarget{}, Environment{}, testLogger(t))
	patch := SettingsPatch{Enabled: boolPointer(true), Policy: gradePolicyString(PolicyAll), GradeLimits: gradePolicyLimits(map[string]int{"A": 1})}
	if _, err := service.Update(context.Background(), patch); err != ErrTargetUnavailable {
		t.Fatalf("unsupported target=%v", err)
	}
	stored, err := NewStore(raw).Load(context.Background())
	if err != nil || stored != nil {
		t.Fatalf("unsupported policy saved=%+v err=%v", stored, err)
	}
}

func gradePolicyString(value string) *string                 { return &value }
func gradePolicyLimits(value map[string]int) *map[string]int { return &value }
