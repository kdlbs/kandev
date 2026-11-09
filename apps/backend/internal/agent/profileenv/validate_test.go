package profileenv

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestValidate(t *testing.T) {
	tooMany := make([]models.ProfileEnvVar, MaxEntries+1)
	for i := range tooMany {
		tooMany[i] = models.ProfileEnvVar{Key: "KEY_" + strings.Repeat("A", i%10) + strings.Repeat("B", i/10), Value: "x"}
	}

	tests := []struct {
		name    string
		entries []models.ProfileEnvVar
		wantErr string
	}{
		{name: "valid literal", entries: []models.ProfileEnvVar{{Key: "FOO", Value: "bar"}}},
		{name: "valid secret", entries: []models.ProfileEnvVar{{Key: "TOKEN", SecretID: "sec-1"}}},
		{name: "maximum entries", entries: tooMany[:MaxEntries]},
		{name: "maximum key length", entries: []models.ProfileEnvVar{{Key: strings.Repeat("K", MaxKeyLen), Value: "x"}}},
		{name: "key too long", entries: []models.ProfileEnvVar{{Key: strings.Repeat("K", MaxKeyLen+1), Value: "x"}}, wantErr: "key exceeds"},
		{name: "maximum value length", entries: []models.ProfileEnvVar{{Key: "FOO", Value: strings.Repeat("x", MaxValueLen)}}},
		{name: "value too long", entries: []models.ProfileEnvVar{{Key: "FOO", Value: strings.Repeat("x", MaxValueLen+1)}}, wantErr: "value exceeds"},
		{name: "whitespace secret id", entries: []models.ProfileEnvVar{{Key: "TOKEN", SecretID: "   "}}, wantErr: "either value or secret_id"},
		{name: "padded secret id", entries: []models.ProfileEnvVar{{Key: "TOKEN", SecretID: " sec-1 "}}, wantErr: "surrounding whitespace"},
		{name: "empty key", entries: []models.ProfileEnvVar{{Key: "", Value: "x"}}, wantErr: "env_vars[0].key is required"},
		{name: "invalid key", entries: []models.ProfileEnvVar{{Key: "BAD KEY", Value: "x"}}, wantErr: "POSIX environment identifier"},
		{name: "reserved key", entries: []models.ProfileEnvVar{{Key: "KANDEV_TASK_ID", Value: "x"}}, wantErr: "is reserved"},
		{name: "duplicate key", entries: []models.ProfileEnvVar{{Key: "FOO", Value: "a"}, {Key: "FOO", Value: "b"}}, wantErr: "duplicates"},
		{name: "both value and secret", entries: []models.ProfileEnvVar{{Key: "FOO", Value: "a", SecretID: "sec-1"}}, wantErr: "not both"},
		{name: "neither value nor secret", entries: []models.ProfileEnvVar{{Key: "FOO"}}, wantErr: "either value or secret_id"},
		{name: "null value", entries: []models.ProfileEnvVar{{Key: "FOO", Value: "bad\x00value"}}, wantErr: "null bytes"},
		{name: "too many entries", entries: tooMany, wantErr: "at most 100 entries"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.entries)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}
