package controller

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func TestValidateEnvVarDTOs_Valid(t *testing.T) {
	t.Parallel()
	cases := map[string][]dto.EnvVarDTO{
		"empty list":           nil,
		"value-only entry":     {{Key: "FOO", Value: "bar"}},
		"secret-ref entry":     {{Key: "FOO", SecretID: "sec-1"}},
		"empty value allowed":  {{Key: "FLAG"}},
		"mixed entries":        {{Key: "A", Value: "1"}, {Key: "B", SecretID: "s"}},
		"keys with underscore": {{Key: "THEONE_ROOT", Value: "/x"}},
		"keys with dash":       {{Key: "MY-FLAG", Value: "y"}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateEnvVarDTOs(in); err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestValidateEnvVarDTOs_Invalid(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		input   []dto.EnvVarDTO
		errSubs string
	}{
		"empty key": {
			input:   []dto.EnvVarDTO{{Key: "", Value: "x"}},
			errSubs: "key is required",
		},
		"whitespace-only key": {
			input:   []dto.EnvVarDTO{{Key: "   ", Value: "x"}},
			errSubs: "key is required",
		},
		"key with equals sign": {
			input:   []dto.EnvVarDTO{{Key: "FOO=BAR", Value: "x"}},
			errSubs: "must not contain '='",
		},
		"key over 256 chars": {
			input:   []dto.EnvVarDTO{{Key: strings.Repeat("A", 257), Value: "x"}},
			errSubs: "exceeds 256 characters",
		},
		"both value and secret id": {
			input:   []dto.EnvVarDTO{{Key: "FOO", Value: "x", SecretID: "s"}},
			errSubs: "only one of value or secret_id",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateEnvVarDTOs(tc.input)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errSubs)
			}
			if !strings.Contains(err.Error(), tc.errSubs) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.errSubs)
			}
		})
	}
}

func TestEnvVarsFromDTO_PreservesOrderAndShape(t *testing.T) {
	t.Parallel()
	in := []dto.EnvVarDTO{
		{Key: "A", Value: "1"},
		{Key: "B", SecretID: "s"},
		{Key: "C"},
	}
	got := envVarsFromDTO(in)
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	if got[0].Key != "A" || got[0].Value != "1" {
		t.Errorf("entry 0 wrong: %+v", got[0])
	}
	if got[1].Key != "B" || got[1].SecretID != "s" {
		t.Errorf("entry 1 wrong: %+v", got[1])
	}
	if got[2].Key != "C" || got[2].Value != "" || got[2].SecretID != "" {
		t.Errorf("entry 2 wrong: %+v", got[2])
	}
}
