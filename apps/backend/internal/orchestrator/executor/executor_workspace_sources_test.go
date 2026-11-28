package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestValidateWorkspaceFoldersForExecutor(t *testing.T) {
	folders := []WorkspaceFolderSpec{{Name: "assets", LocalPath: t.TempDir()}}
	tests := []struct {
		name         string
		executorType string
		wantErr      bool
	}{
		{name: "local", executorType: string(models.ExecutorTypeLocal)},
		{name: "local pc", executorType: "local_pc"},
		{name: "worktree", executorType: string(models.ExecutorTypeWorktree)},
		{name: "docker", executorType: string(models.ExecutorTypeLocalDocker), wantErr: true},
		{name: "ssh", executorType: string(models.ExecutorTypeSSH), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkspaceFoldersForExecutor(tt.executorType, folders)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateWorkspaceFoldersForExecutor(%q) error = %v, wantErr %t", tt.executorType, err, tt.wantErr)
			}
		})
	}
}

func TestValidateWorkspaceFoldersForExecutorAllowsEmptyFolders(t *testing.T) {
	if err := validateWorkspaceFoldersForExecutor(string(models.ExecutorTypeLocalDocker), nil); err != nil {
		t.Fatalf("empty workspace folders should be accepted: %v", err)
	}
}
