package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestIsExecutorRunningNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "sentinel error directly",
			err:  models.ErrExecutorRunningNotFound,
			want: true,
		},
		{
			name: "wrapped sentinel error",
			err:  fmt.Errorf("%w for session: abc-123", models.ErrExecutorRunningNotFound),
			want: true,
		},
		{
			name: "double-wrapped sentinel error",
			err:  fmt.Errorf("outer: %w", fmt.Errorf("%w for session: abc-123", models.ErrExecutorRunningNotFound)),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection refused"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExecutorRunningNotFoundError(tt.err)
			if got != tt.want {
				t.Errorf("isExecutorRunningNotFoundError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
