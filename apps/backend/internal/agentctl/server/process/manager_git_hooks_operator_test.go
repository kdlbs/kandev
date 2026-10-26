package process

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestGitOperatorsTakeHooksDecisionFromTheLaunchSnapshot(t *testing.T) {
	tmp := t.TempDir()
	for _, name := range []string{"front", "back"} {
		if err := os.MkdirAll(filepath.Join(tmp, name), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	mgr := NewManager(&config.InstanceConfig{WorkDir: tmp, GitHooks: map[string]bool{"front": true}}, log)

	front, err := mgr.GitOperatorFor("front")
	if err != nil {
		t.Fatalf("front: %v", err)
	}
	back, err := mgr.GitOperatorFor("back")
	if err != nil {
		t.Fatalf("back: %v", err)
	}
	if !front.gitHooks {
		t.Error("the opted-in repository must run hooks")
	}
	if back.gitHooks {
		t.Error("a repository the map lacks must keep hooks off")
	}
	if mgr.GitOperator().gitHooks {
		t.Error("the root operator must not inherit another entry's decision")
	}
}

func TestRootGitOperatorUsesTheEmptyKey(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), GitHooks: map[string]bool{"": true}}, log)
	if !mgr.GitOperator().gitHooks {
		t.Error("the root key must enable hooks for the root operator")
	}
}
