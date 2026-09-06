package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/worktree"
	"go.uber.org/zap"
)

const maxGitMetadataDiagnosticRepositories = 8

// logGitMetadataPolicyInstalled emits one bounded, path-free policy record.
// Projection hashes are freshness diagnostics; neither source paths nor the
// rendered agent configuration are included in structured fields.
func (m *Manager) logGitMetadataPolicyInstalled(
	taskID, taskEnvironmentID, executorType string,
	repositories []WorkspaceRepositorySpec,
	req *ExecutorCreateRequest,
	runtime ExecutorBackend,
) {
	if m == nil || m.logger == nil || req == nil || runtime == nil ||
		(len(req.GitMetadataProjections) == 0 && !requiresCloneGitMetadataPolicy(req)) {
		return
	}
	strategy := "linked_worktree_projection"
	if requiresCloneGitMetadataPolicy(req) {
		strategy = "clone_attestation"
	}
	repositoryIDs := make([]string, 0, min(len(repositories), maxGitMetadataDiagnosticRepositories))
	for _, repository := range repositories {
		if repository.RepositoryID == "" || len(repositoryIDs) == maxGitMetadataDiagnosticRepositories {
			continue
		}
		repositoryIDs = append(repositoryIDs, repository.RepositoryID)
	}
	m.logger.Info("Git metadata filesystem policy installed",
		zap.String("task_id", taskID),
		zap.String("task_environment_id", taskEnvironmentID),
		zap.String("executor_type", executorType),
		zap.Stringer("runtime", runtime.Name()),
		zap.Int("repository_count", len(repositories)),
		zap.Strings("repository_ids", repositoryIDs),
		zap.Int("projection_version", worktree.GitMetadataProjectionVersion),
		zap.String("projection_hash", gitMetadataPolicyDiagnosticHash(req)),
		zap.String("enforcement_strategy", strategy))
}

func gitMetadataPolicyDiagnosticHash(req *ExecutorCreateRequest) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "v%d\n", worktree.GitMetadataProjectionVersion)
	for _, projection := range req.GitMetadataProjections {
		if projection != nil {
			_, _ = fmt.Fprintf(hash, "%d:%s\n", projection.Version, projection.Hash)
		}
	}
	if len(req.GitMetadataProjections) == 0 {
		if keysAgent, ok := req.AgentConfig.(agents.FilesystemPolicyEnvironmentAgent); ok {
			keys := append([]string(nil), keysAgent.FilesystemPolicyEnvironmentKeys()...)
			sort.Strings(keys)
			for _, key := range keys {
				_, _ = fmt.Fprintf(hash, "%s\x00%s\n", key, req.Env[key])
			}
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}
