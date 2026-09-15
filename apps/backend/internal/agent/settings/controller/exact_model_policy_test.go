package controller

import (
	"errors"
	"testing"
)

func TestValidateRequireExactModelPolicy(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		require     bool
		passthrough bool
		dynamic     bool
		wantErr     error
	}{
		{name: "compatible empty", wantErr: nil},
		{name: "strict concrete", model: "provider/model", require: true, wantErr: nil},
		{name: "strict empty", require: true, wantErr: ErrRequireExactModelNeedsModel},
		{name: "strict passthrough", model: "provider/model", require: true, passthrough: true, wantErr: ErrRequireExactModelUnsupported},
		{name: "strict dynamic", model: "provider/model", require: true, dynamic: true, wantErr: ErrRequireExactModelUnsupported},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequireExactModelPolicy(tt.model, tt.require, tt.passthrough, tt.dynamic)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
