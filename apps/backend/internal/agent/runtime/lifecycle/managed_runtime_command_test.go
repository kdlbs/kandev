package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/task/models"
)

type managedRuntimeSelectionStore struct {
	selection managedruntime.Selection
	found     bool
	err       error
}

func (s managedRuntimeSelectionStore) Get(
	context.Context,
	string,
	string,
) (managedruntime.Selection, bool, error) {
	return s.selection, s.found, s.err
}

func (s managedRuntimeSelectionStore) Save(context.Context, string, string, string) error {
	return nil
}

func TestBuildAgentCommandUsesExactVersionOnlyOnStandalone(t *testing.T) {
	log := newTestLogger()
	manager := &Manager{commandBuilder: NewCommandBuilder(), logger: log}
	manager.SetManagedRuntimeSelectionStore(managedRuntimeSelectionStore{
		selection: managedruntime.Selection{Package: "opencode-ai", Version: "1.18.5"},
		found:     true,
	})
	agent := agents.NewOpenCodeACP()

	tests := []struct {
		name         string
		executorType models.ExecutorType
		wantVersion  bool
	}{
		{name: "standalone", executorType: models.ExecutorTypeLocal, wantVersion: true},
		{name: "worktree", executorType: models.ExecutorTypeWorktree, wantVersion: true},
		{name: "docker", executorType: models.ExecutorTypeLocalDocker},
		{name: "ssh", executorType: models.ExecutorTypeSSH},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmds, err := manager.buildAgentCommandWithContext(
				context.Background(),
				&LaunchRequest{ExecutorType: string(tt.executorType)},
				nil,
				agent,
				false,
			)
			if err != nil {
				t.Fatalf("buildAgentCommandWithContext: %v", err)
			}
			hasVersion := strings.Contains(cmds.initial, "opencode-ai@1.18.5")
			if hasVersion != tt.wantVersion {
				t.Fatalf("command = %q, version present = %v, want %v", cmds.initial, hasVersion, tt.wantVersion)
			}
		})
	}
}

func TestBuildAgentCommandFailsWhenActiveSelectionCannotBeRead(t *testing.T) {
	manager := &Manager{commandBuilder: NewCommandBuilder(), logger: newTestLogger()}
	wantErr := errors.New("selection unavailable")
	manager.SetManagedRuntimeSelectionStore(managedRuntimeSelectionStore{err: wantErr})

	_, err := manager.buildAgentCommandWithContext(
		context.Background(),
		&LaunchRequest{ExecutorType: string(models.ExecutorTypeLocal)},
		nil,
		agents.NewOpenCodeACP(),
		false,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("selection error = %v, want %v", err, wantErr)
	}
}
