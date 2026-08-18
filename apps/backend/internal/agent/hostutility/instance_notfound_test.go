package hostutility

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsInstanceNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{
			// The exact shape ControlClient.DeleteInstance produces on shutdown.
			name: "delete instance 404",
			err:  fmt.Errorf("failed to delete instance: instance not found (status 404)"),
			want: true,
		},
		{
			name: "not found phrase without status",
			err:  errors.New("instance \"inst-3\" not found"),
			want: true,
		},
		{
			name: "status 404 uppercased",
			err:  errors.New("DELETE returned STATUS 404"),
			want: true,
		},
		{
			name: "transport failure is not not-found",
			err:  errors.New("failed to delete instance: connection refused"),
			want: false,
		},
		{
			name: "server error is not not-found",
			err:  errors.New("failed to delete instance: internal error (status 500)"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInstanceNotFound(tt.err); got != tt.want {
				t.Fatalf("isInstanceNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
