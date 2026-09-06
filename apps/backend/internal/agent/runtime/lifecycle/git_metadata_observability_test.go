package lifecycle

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestGitMetadataPolicyObservabilityIsBoundedAndPathFree(t *testing.T) {
	core, observed := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	mgr := &Manager{logger: log}
	repositories := make([]WorkspaceRepositorySpec, maxGitMetadataDiagnosticRepositories+2)
	for index := range repositories {
		repositories[index] = WorkspaceRepositorySpec{RepositoryID: "repository-" + string(rune('a'+index))}
	}
	req := &ExecutorCreateRequest{
		GitMetadataRequirement: cloneGitMetadataRequirement(true),
		AgentConfig:            agents.NewCodexACP(),
		Env: map[string]string{
			"CODEX_CONFIG": `{"permissions":{"task":{"filesystem":{"/sensitive/source/.git":"write"}}}}`,
		},
	}

	mgr.logGitMetadataPolicyInstalled("task-1", "environment-1", "local_docker", repositories, req, &lazyCloneExecutor{})

	entries := observed.FilterMessage("Git metadata filesystem policy installed").All()
	if len(entries) != 1 {
		t.Fatalf("diagnostic log entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["projection_version"] != int64(2) || fields["projection_hash"] == "" {
		t.Fatalf("projection diagnostics = %#v, want version and hash", fields)
	}
	ids, ok := fields["repository_ids"].([]interface{})
	if !ok || len(ids) != maxGitMetadataDiagnosticRepositories {
		t.Fatalf("repository_ids = %#v, want bounded list of %d", fields["repository_ids"], maxGitMetadataDiagnosticRepositories)
	}
	if strings.Contains(entries[0].Message+fields["projection_hash"].(string), "/sensitive/") {
		t.Fatalf("diagnostic log disclosed an absolute source path: %#v", fields)
	}
}
