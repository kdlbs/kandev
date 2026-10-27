//go:build linux

package sandbox

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSandboxPoliciesCannotGrantProcessContainmentControl(t *testing.T) {
	for _, path := range []string{"/sys", "/sys/fs/cgroup"} {
		_, err := Canonicalize(Grants{Filesystem: []FilesystemGrant{{Path: path, Writable: true}}}, nil)
		require.Error(t, err, "kernel process membership must not be mutable by a sandbox")
	}
	_, err := ResolveWorkDirectory(Grants{Filesystem: []FilesystemGrant{{Reference: ReferenceWorkDir, Writable: true}}}, "/sys/fs/cgroup", nil)
	require.Error(t, err)
}
