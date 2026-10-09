package managedruntime

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrepareNPMProjectPrefixUsesPrivateTemporaryRoot(t *testing.T) {
	tempRoot := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("TEMP", tempRoot)
		t.Setenv("TMP", tempRoot)
	} else {
		t.Setenv("TMPDIR", tempRoot)
	}
	agentHome := filepath.Join(t.TempDir(), "mounted-agent-home")
	if err := os.MkdirAll(agentHome, 0o700); err != nil {
		t.Fatalf("create mounted agent home: %v", err)
	}

	args := append([]string{"npx"}, NPMProjectPrefixArgs()...)
	if err := PrepareNPMProjectPrefix(args); err != nil {
		t.Fatalf("prepare npm project prefix: %v", err)
	}

	got := args[2]
	if !filepath.IsAbs(got) || !strings.HasPrefix(got, filepath.Clean(tempRoot)+string(filepath.Separator)) {
		t.Fatalf("npm project prefix = %q, want absolute path under system temp root %q", got, tempRoot)
	}
	info, err := os.Stat(got)
	if err != nil || !info.IsDir() {
		t.Fatalf("npm project prefix stat = (%v, %v), want an existing directory", info, err)
	}
	if runtime.GOOS != "windows" {
		if gotMode := info.Mode().Perm(); gotMode != 0o700 {
			t.Fatalf("npm project prefix permissions = %#o, want %#o", gotMode, 0o700)
		}
	}

	legacyHomePrefix := filepath.Join(agentHome, ".kandev", "managed-npm-runtime")
	if _, err := os.Stat(legacyHomePrefix); !os.IsNotExist(err) {
		t.Fatalf("npm project prefix was created under mounted agent home, stat error = %v", err)
	}
}
