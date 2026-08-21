package service

import (
	"strings"
	"testing"
)

func TestEncodeTaskLabelsPreservesCreateEmptySemantics(t *testing.T) {
	if got := EncodeTaskLabels([]string{" ", ""}); got != "" {
		t.Errorf("EncodeTaskLabels(empty) = %q, want empty string", got)
	}
	if got := EncodeTaskLabels([]string{" bug ", "", "bug", "plugin", "plugin"}); got != `["bug","plugin"]` {
		t.Errorf("EncodeTaskLabels() = %q, want normalized labels", got)
	}
}

func TestEncodeTaskLabelsForUpdateClearsWithEmptyArray(t *testing.T) {
	if got := EncodeTaskLabelsForUpdate([]string{" ", ""}); got != `[]` {
		t.Errorf("EncodeTaskLabelsForUpdate(empty) = %q, want []", got)
	}
}

func TestValidateTaskLabels(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
		wantErr string
	}{
		{name: "empty string", encoded: "", wantErr: ""},
		{name: "empty array", encoded: "[]", wantErr: ""},
		{name: "valid labels", encoded: `["bug","security"]`, wantErr: ""},
		{name: "not JSON", encoded: "not-json", wantErr: "invalid labels JSON"},
		{name: "non-array JSON", encoded: `{"key":"val"}`, wantErr: "invalid labels JSON"},
		{name: "array with non-string element", encoded: `["ok", 42]`, wantErr: "invalid labels JSON"},
		{name: "blank label", encoded: `["ok", "  "]`, wantErr: "must not be blank"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTaskLabels(tt.encoded)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateTaskLabels(%q) = %v, want nil", tt.encoded, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateTaskLabels(%q) = nil, want error containing %q", tt.encoded, tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ValidateTaskLabels(%q) = %v, want error containing %q", tt.encoded, err, tt.wantErr)
				}
			}
		})
	}
}
