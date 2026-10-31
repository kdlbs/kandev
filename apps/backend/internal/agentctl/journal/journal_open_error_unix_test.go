//go:build unix

package journal

import (
	"os"
	"syscall"
	"testing"
)

func TestClassifyOpenFailureUsesTypedFilesystemCauses(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want OpenReason
	}{
		{"filesystem full", &os.PathError{Op: "write", Path: "/journal", Err: syscall.ENOSPC}, OpenReasonFilesystemFull},
		{"disk quota", &os.PathError{Op: "write", Path: "/journal", Err: syscall.EDQUOT}, OpenReasonDiskQuotaExceeded},
		{"io failure", &os.PathError{Op: "read", Path: "/journal", Err: syscall.EIO}, OpenReasonIOFailure},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyOpenFailure(tc.err); got != tc.want {
				t.Fatalf("classifyOpenFailure(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
