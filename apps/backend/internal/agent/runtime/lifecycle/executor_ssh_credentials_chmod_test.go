package lifecycle

import (
	"errors"
	"testing"

	"github.com/pkg/sftp"
)

// Win32-OpenSSH cannot apply POSIX modes, so only a Windows path may skip an
// unsupported chmod. On Linux or macOS a credential file must not silently
// keep the server's default (often world-readable) mode.
func TestSSHSFTPChmodFailureIsToleratedOnlyForWindowsPaths(t *testing.T) {
	unsupported := &sftp.StatusError{Code: uint32(sftp.ErrSSHFxOpUnsupported)}
	cases := map[string]struct {
		path string
		err  error
		want bool
	}{
		"windows unsupported": {path: `C:\Users\k\.kandev\creds.json`, err: unsupported, want: true},
		"posix unsupported":   {path: "/home/k/.kandev/creds.json", err: unsupported},
		"windows other error": {path: `C:\Users\k\.kandev\creds.json`, err: errors.New("permission denied")},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := sftpChmodFailureTolerated(tc.path, tc.err); got != tc.want {
				t.Fatalf("sftpChmodFailureTolerated(%q, %v) = %v, want %v", tc.path, tc.err, got, tc.want)
			}
		})
	}
}
