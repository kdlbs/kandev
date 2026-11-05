package service

import (
	"errors"
	"expvar"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func contractProvider(declares bool) models.ExecutorProvider {
	provider := models.ExecutorProvider{ProfileSchema: map[string]any{
		"type":     "object",
		"required": []any{"region", "name"},
		"properties": map[string]any{
			"region":  map[string]any{"type": "string", "default": "eu-west", "pattern": "[a-z]+-[a-z]+"},
			"name":    map[string]any{"type": "string", "minLength": 1, "maxLength": 8},
			"cpus":    map[string]any{"type": "integer", "default": 2, "minimum": 1, "maximum": 8},
			"spot":    map[string]any{"type": "boolean"},
			"note":    map[string]any{"type": "string", "minLength": 0},
			"api_key": map[string]any{"type": "string", "secret": true},
		},
	}}
	if declares {
		provider.RequiredCapabilities = []models.ExecutorProviderCapabilityRequirement{{Name: "typed_profile_values", Version: 1}}
	}
	return provider
}

func fieldCodes(t *testing.T, err error) map[string]string {
	t.Helper()
	var invalid *ProfileFieldErrors
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want *ProfileFieldErrors", err)
	}
	if !errors.Is(err, ErrInvalidExecutorConfig) {
		t.Fatalf("err = %v must wrap ErrInvalidExecutorConfig", err)
	}
	codes := map[string]string{}
	for _, item := range invalid.Errors {
		codes[item.Field] = item.Code
	}
	return codes
}

func TestValidateProfileFillsDefaultsForAbsentAndEmptyValues(t *testing.T) {
	filled, err := fillPluginExecutorProfile(contractProvider(false), map[string]string{"name": "box", "region": ""})
	if err != nil {
		t.Fatalf("fill: %v", err)
	}
	if filled["region"] != "eu-west" || filled["cpus"] != "2" {
		t.Fatalf("filled = %v, want defaults applied to the empty and the absent property", filled)
	}
	if _, present := filled["spot"]; present {
		t.Fatalf("a property without a default must stay absent: %v", filled)
	}
}

func TestValidateProfileReturnsStableFieldErrors(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]string
		field  string
		code   string
	}{
		{"required", map[string]string{}, "name", "required"},
		{"empty required without default", map[string]string{"name": ""}, "name", "required"},
		{"too long", map[string]string{"name": "123456789"}, "name", "too_long"},
		{"pattern", map[string]string{"name": "ok", "region": "EU"}, "region", "pattern_mismatch"},
		{"unparseable integer", map[string]string{"name": "ok", "cpus": "many"}, "cpus", "invalid_type"},
		{"out of range", map[string]string{"name": "ok", "cpus": "99"}, "cpus", "out_of_range"},
		{"bad bool", map[string]string{"name": "ok", "spot": "yes"}, "spot", "invalid_type"},
		{"undeclared", map[string]string{"name": "ok", "bogus": "1"}, "bogus", "unknown_property"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fillPluginExecutorProfile(contractProvider(true), tc.config)
			if got := fieldCodes(t, err)[tc.field]; got != tc.code {
				t.Fatalf("codes = %v, want %s=%s", fieldCodes(t, err), tc.field, tc.code)
			}
		})
	}
}

func TestValidateProfileKeepsAnExplicitEmptyString(t *testing.T) {
	filled, err := fillPluginExecutorProfile(contractProvider(false), map[string]string{"name": "ok", "note": ""})
	if err != nil {
		t.Fatalf("fill: %v", err)
	}
	if value, present := filled["note"]; !present || value != "" {
		t.Fatalf("an explicit minLength 0 string must keep its empty value: %v", filled)
	}
}

func TestValidateProfileErrorsNeverEchoValues(t *testing.T) {
	provider := contractProvider(true)
	secret := "super-secret-token-value"
	_, err := fillPluginExecutorProfile(provider, map[string]string{"name": secret + secret, "api_key": secret})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error echoes a value: %v", err)
	}
}

func TestHostOwnedKeysAreExemptOnlyForDeclaringProviders(t *testing.T) {
	config := map[string]string{"name": "ok", "launch_policy": "x", "hint.size": "big", "remote_auth_target_home": "/root"}
	if _, err := fillPluginExecutorProfile(contractProvider(true), config); err != nil {
		t.Fatalf("declaring provider must accept the host-owned keys: %v", err)
	}
	_, err := fillPluginExecutorProfile(contractProvider(false), config)
	if got := fieldCodes(t, err)["launch_policy"]; got != "unknown_property" {
		t.Fatalf("a non-declaring provider keeps rejecting undeclared keys, got %v", fieldCodes(t, err))
	}
}

func TestProfileTooLargeAppliesOnlyToDeclaringProviders(t *testing.T) {
	big := map[string]string{"name": "ok", "hint.a": strings.Repeat("x", 3000), "hint.b": strings.Repeat("y", 1200)}
	_, err := fillPluginExecutorProfile(contractProvider(true), big)
	if got := fieldCodes(t, err)[""]; got != "profile_too_large" {
		t.Fatalf("codes = %v, want a profile-level profile_too_large", fieldCodes(t, err))
	}
	if _, err := fillPluginExecutorProfile(contractProvider(false), map[string]string{"name": "ok"}); err != nil {
		t.Fatalf("non-declaring provider: %v", err)
	}
}

func TestProfileSizeMeasureIsTheSharedFunctionAtSaveAndLaunch(t *testing.T) {
	config := map[string]string{"name": "ok", "region": "eu-west", "cpus": "2", "remote_auth_target_home": strings.Repeat("h", 500)}
	filled, err := fillPluginExecutorProfile(contractProvider(true), config)
	if err != nil {
		t.Fatalf("fill: %v", err)
	}
	want := models.ExecutorProfileContractBytes(map[string]string{"name": "ok", "region": "eu-west", "cpus": "2"}, nil)
	if got := pluginExecutorProfileContractBytes(contractProvider(true), filled); got != want {
		t.Fatalf("measure = %d, want %d (the six kandev keys are excluded)", got, want)
	}
}

func TestContractBytesCountsSecretIDsAsACompactArray(t *testing.T) {
	base := models.ExecutorProfileContractBytes(map[string]string{"a": "b"}, nil)
	withIDs := models.ExecutorProfileContractBytes(map[string]string{"a": "b"}, []string{"id-1", "id-2"})
	if withIDs-base != len(`["id-1","id-2"]`) {
		t.Fatalf("secret id array adds %d bytes, want %d", withIDs-base, len(`["id-1","id-2"]`))
	}
}

func TestLegacyValidationStillRejectsWithoutProfileFieldErrorsLeak(t *testing.T) {
	if err := validatePluginExecutorProfileSchema(contractProvider(false), map[string]string{"name": "ok", "cpus": "x"}); !errors.Is(err, ErrInvalidExecutorConfig) {
		t.Fatalf("err = %v", err)
	}
}

func TestProfileInvalidMetricCountsByCodeOnly(t *testing.T) {
	before := executorPluginProfileInvalidTotal.Get("required")
	var beforeValue int64
	if before != nil {
		beforeValue = before.(*expvar.Int).Value()
	}
	if _, err := fillPluginExecutorProfile(contractProvider(false), map[string]string{}); err == nil {
		t.Fatal("expected a required error")
	}
	after := executorPluginProfileInvalidTotal.Get("required").(*expvar.Int).Value()
	if after != beforeValue+1 {
		t.Fatalf("counter = %d, want %d", after, beforeValue+1)
	}
	executorPluginProfileInvalidTotal.Do(func(kv expvar.KeyValue) {
		if strings.Contains(kv.Key, "name") || strings.Contains(kv.Key, "region") {
			t.Fatalf("metric label %q carries a property name", kv.Key)
		}
	})
}
