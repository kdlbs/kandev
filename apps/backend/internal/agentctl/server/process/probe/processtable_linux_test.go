//go:build linux

package probe

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestIsLinuxProcessGone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "missing proc entry",
			err:  &os.PathError{Op: "read", Path: "/proc/123/stat", Err: syscall.ENOENT},
			want: true,
		},
		{
			name: "process exited during read",
			err:  &os.PathError{Op: "read", Path: "/proc/123/stat", Err: syscall.ESRCH},
			want: true,
		},
		{
			name: "permission failure",
			err:  &os.PathError{Op: "read", Path: "/proc/123/stat", Err: syscall.EACCES},
			want: false,
		},
		{
			name: "unrelated failure",
			err:  errors.New("read failed"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isLinuxProcessGone(tt.err); got != tt.want {
				t.Fatalf("isLinuxProcessGone() = %v, want %v", got, tt.want)
			}
		})
	}
}
