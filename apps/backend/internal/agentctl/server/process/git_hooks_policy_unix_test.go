//go:build unix

package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestManagerGitHooksEnabledIsAnExactLookup(t *testing.T) {
	m := &Manager{cfg: &config.InstanceConfig{GitHooks: map[string]bool{"": true, "front": true}}}
	for key, want := range map[string]bool{"": true, "front": true, "back": false, "front/sub": false} {
		if got := m.gitHooksEnabled(key); got != want {
			t.Errorf("gitHooksEnabled(%q) = %t, want %t", key, got, want)
		}
	}
	if (&Manager{cfg: &config.InstanceConfig{}}).gitHooksEnabled("") {
		t.Error("an empty snapshot must leave hooks off")
	}
	if (&Manager{}).gitHooksEnabled("") {
		t.Error("a manager without config must leave hooks off")
	}
}

func TestGitOperatorOnlyRunsHooksForAnOptedInRepository(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	shim := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"" + argsPath + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(shim), 0o700); err != nil {
		t.Fatalf("write git shim: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	for _, tc := range []struct {
		name       string
		hooks      bool
		wantPinned bool
	}{{"hooks off pins the hooks path", false, true}, {"hooks on leaves the repository hooks", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.Remove(argsPath)
			op := NewGitOperator(dir, log, nil)
			op.gitHooks = tc.hooks
			if _, err := op.runGitCommandWithEnvironment(context.Background(), nil, "status"); err != nil {
				t.Fatalf("run git: %v", err)
			}
			data, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatalf("read recorded args: %v", err)
			}
			if pinned := strings.Contains(string(data), "core.hooksPath="); pinned != tc.wantPinned {
				t.Fatalf("core.hooksPath pinned = %t, want %t in %q", pinned, tc.wantPinned, data)
			}
		})
	}
}
