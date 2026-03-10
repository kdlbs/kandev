package service

import (
	"database/sql"
	"errors"
	"testing"
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
			name: "sql.ErrNoRows",
			err:  sql.ErrNoRows,
			want: true,
		},
		{
			name: "wrapped sql.ErrNoRows",
			err:  errors.Join(errors.New("outer"), sql.ErrNoRows),
			want: true,
		},
		{
			name: "error containing not-found message",
			err:  errors.New("executor running not found for session abc-123"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection refused"),
			want: false,
		},
		{
			name: "partial match should not match",
			err:  errors.New("executor running found"),
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
