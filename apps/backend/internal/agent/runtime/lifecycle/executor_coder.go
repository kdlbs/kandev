package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
)

var nonCoderWorkspaceChar = regexp.MustCompile(`[^a-z0-9-]+`)

type coderCommand func(context.Context, string, ...string) ([]byte, error)

// CoderWorkspaceManager owns the create/start/readiness boundary. It never
// reads or stores a Coder token: the configured CLI profile is the credential
// boundary, matching normal interactive `coder` usage.
type CoderWorkspaceManager struct {
	run coderCommand
}

func newCoderWorkspaceManager() *CoderWorkspaceManager {
	return &CoderWorkspaceManager{run: runCoderCommand}
}

func runCoderCommand(ctx context.Context, binary string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", binary, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

type coderWorkspaceListRow struct {
	Name        string `json:"name"`
	LatestBuild struct {
		Status string `json:"status"`
	} `json:"latest_build"`
}

func (m *CoderWorkspaceManager) Ensure(ctx context.Context, req *ExecutorCreateRequest) (string, error) {
	if req == nil {
		return "", errors.New("coder executor: create request is required")
	}
	binary := getMetadataString(req.Metadata, MetadataKeyCoderBinary)
	if binary == "" {
		binary = string(SSHIdentitySourceCoder)
	}
	workspace := resolveCoderWorkspace(req)
	template := getMetadataString(req.Metadata, MetadataKeyCoderTemplate)

	found, status, err := m.workspaceState(ctx, binary, workspace)
	if err != nil {
		return "", err
	}
	if !found {
		if template == "" {
			return "", errors.New("coder executor: template is required when creating a workspace")
		}
		if _, err := m.run(ctx, binary, "create", "--yes", "--template", template, "--use-parameter-defaults", workspace); err != nil {
			return "", fmt.Errorf("coder executor: create workspace %q: %w", workspace, err)
		}
	} else if status == "stopped" || status == "failed" || status == "canceled" {
		if _, err := m.run(ctx, binary, "start", "--yes", "--use-parameter-defaults", workspace); err != nil {
			return "", fmt.Errorf("coder executor: start workspace %q: %w", workspace, err)
		}
	}
	if _, err := m.run(ctx, binary, "ssh", "--wait", "yes", workspace, "--", "true"); err != nil {
		return "", fmt.Errorf("coder executor: workspace %q did not become ready: %w", workspace, err)
	}
	return workspace, nil
}

func resolveCoderWorkspace(req *ExecutorCreateRequest) string {
	if workspace := getMetadataString(req.Metadata, MetadataKeyCoderWorkspace); workspace != "" {
		return workspace
	}
	prefix := getMetadataString(req.Metadata, MetadataKeyCoderWorkspacePrefix)
	if prefix == "" {
		prefix = "kandev"
	}
	return coderWorkspaceName(prefix, req.TaskID)
}

func (m *CoderWorkspaceManager) workspaceState(ctx context.Context, binary, workspace string) (bool, string, error) {
	out, err := m.run(ctx, binary, "list", "--output", "json", "--search", "owner:me name:"+workspace)
	if err != nil {
		return false, "", fmt.Errorf("coder executor: list workspaces: %w", err)
	}
	var rows []coderWorkspaceListRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return false, "", fmt.Errorf("coder executor: decode workspace list: %w", err)
	}
	for _, row := range rows {
		if row.Name == workspace {
			return true, row.LatestBuild.Status, nil
		}
	}
	return false, "", nil
}

func coderWorkspaceName(prefix, taskID string) string {
	prefix = nonCoderWorkspaceChar.ReplaceAllString(strings.ToLower(strings.TrimSpace(prefix)), "-")
	prefix = strings.Trim(prefix, "-")
	if prefix == "" {
		prefix = "kandev"
	}
	id := nonCoderWorkspaceChar.ReplaceAllString(strings.ToLower(taskID), "-")
	id = strings.Trim(id, "-")
	if len(id) > 12 {
		id = id[:12]
	}
	if id == "" {
		id = "task"
	}
	return prefix + "-" + id
}

// NewCoderExecutor uses the proven SSH agentctl lifecycle while replacing the
// transport and host lifecycle with Coder-native operations.
func NewCoderExecutor(secretStore secrets.SecretStore, agentList RemoteAuthAgentLister, resolver *AgentctlResolver, log *logger.Logger) *SSHExecutor {
	r := NewSSHExecutor(secretStore, agentList, resolver, log)
	r.name = executor.NameCoder
	r.coder = newCoderWorkspaceManager()
	r.logger = log.WithFields(zap.String("runtime", string(SSHIdentitySourceCoder)))
	return r
}
