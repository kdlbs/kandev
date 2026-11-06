package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/passthroughconfig"
)

// ErrProjectMCPConflict reports a stale preparation generation or a file
// claimed by another execution.
var ErrProjectMCPConflict = errors.New("project MCP preparation conflict")

// ErrProjectMCPInvalid reports malformed client preparation input.
var ErrProjectMCPInvalid = errors.New("invalid project MCP preparation request")

const (
	maxProjectMCPFiles      = 32
	maxProjectMCPFileBytes  = 1 << 20
	maxProjectMCPTotalBytes = 4 << 20
)

// ProjectMCPFilesOwner owns executor-local project configuration files for one
// agent instance. Files are materialized under the instance workspace through
// os.Root containment and tracked by the shared passthrough ownership registry.
type ProjectMCPFilesOwner struct {
	manager    *Manager
	mu         sync.Mutex
	generation uint64
}

func newProjectMCPFilesOwner(manager *Manager) *ProjectMCPFilesOwner {
	return &ProjectMCPFilesOwner{manager: manager}
}

// ProjectMCPFiles exposes this instance's project MCP file owner.
func (m *Manager) ProjectMCPFiles() *ProjectMCPFilesOwner {
	if m == nil {
		return nil
	}
	return m.projectMCP
}

func (o *ProjectMCPFilesOwner) Target() types.ProjectMCPTarget {
	cfg := o.manager.cfg
	o.mu.Lock()
	generation := o.generation
	o.mu.Unlock()
	return types.ProjectMCPTarget{
		Version:       types.ProjectMCPFilesVersion,
		InstanceID:    cfg.InstanceID,
		SessionID:     cfg.SessionID,
		WorkspacePath: cfg.WorkDir,
		MCPURL:        agentctlMCPServerURL(cfg.Port, "/mcp"),
		Generation:    generation,
	}
}

func agentctlMCPServerURL(port int, path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
}

// Prepare materializes generated files inside the helper workspace. It accepts
// one monotonic generation per materialization and returns read-back
// fingerprints so the backend can require proof before emitting evidence.
func (o *ProjectMCPFilesOwner) Prepare(
	ctx context.Context,
	request types.ProjectMCPPrepareRequest,
) (types.ProjectMCPPrepareResponse, error) {
	m := o.manager
	m.startMu.Lock()
	defer m.startMu.Unlock()
	ctx, release, err := m.BeginOwnedOperation(ctx)
	if err != nil {
		return types.ProjectMCPPrepareResponse{}, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return types.ProjectMCPPrepareResponse{}, err
	}
	cfg := m.cfg
	if cfg.SessionID == "" || filepath.Base(cfg.SessionID) != cfg.SessionID {
		return types.ProjectMCPPrepareResponse{}, ErrProjectMCPConflict
	}
	if err := validateProjectMCPPrepareRequest(request); err != nil {
		return types.ProjectMCPPrepareResponse{}, err
	}
	o.mu.Lock()
	current := o.generation
	o.mu.Unlock()
	if request.Generation == 0 || request.Generation != current+1 {
		return types.ProjectMCPPrepareResponse{}, ErrProjectMCPConflict
	}
	if len(request.Files) > 0 {
		privateRoot := filepath.Join(cfg.WorkDir, ".kandev", "sessions", cfg.SessionID, "project-mcp")
		if err := passthroughconfig.Prepare(ctx, cfg.WorkDir, privateRoot, o.owner(), request.Files); err != nil {
			return types.ProjectMCPPrepareResponse{}, err
		}
	}
	evidence, err := o.readBack(request.Files)
	if err != nil {
		_ = passthroughconfig.Cleanup(context.WithoutCancel(ctx), cfg.WorkDir, o.owner(), false)
		return types.ProjectMCPPrepareResponse{}, err
	}
	o.mu.Lock()
	o.generation = request.Generation
	o.mu.Unlock()
	target := o.Target()
	return types.ProjectMCPPrepareResponse{
		Version:       types.ProjectMCPFilesVersion,
		InstanceID:    target.InstanceID,
		SessionID:     target.SessionID,
		WorkspacePath: target.WorkspacePath,
		MCPURL:        target.MCPURL,
		Generation:    target.Generation,
		Files:         evidence,
	}, nil
}

// Cleanup releases this instance's project-file claims. Files owned by another
// execution or by the user remain untouched.
func (o *ProjectMCPFilesOwner) Cleanup(ctx context.Context) error {
	m := o.manager
	ctx, release, err := m.BeginOwnedOperation(ctx)
	if err != nil {
		return err
	}
	defer release()
	cfg := m.cfg
	if cfg.SessionID == "" {
		return nil
	}
	if err := passthroughconfig.Cleanup(ctx, cfg.WorkDir, o.owner(), false); err != nil {
		return err
	}
	o.mu.Lock()
	o.generation = 0
	o.mu.Unlock()
	return nil
}

func validateProjectMCPPrepareRequest(request types.ProjectMCPPrepareRequest) error {
	if request.Version != types.ProjectMCPFilesVersion || len(request.Files) > maxProjectMCPFiles {
		return ErrProjectMCPInvalid
	}
	totalBytes := 0
	for _, file := range request.Files {
		if file.Path == "" || len(file.Content) > maxProjectMCPFileBytes {
			return ErrProjectMCPInvalid
		}
		totalBytes += len(file.Content)
	}
	if totalBytes > maxProjectMCPTotalBytes {
		return ErrProjectMCPInvalid
	}
	return nil
}

func (o *ProjectMCPFilesOwner) owner() string {
	cfg := o.manager.cfg
	return cfg.InstanceID + ":" + cfg.SessionID + ":project"
}

// readBack confirms every requested file is a regular file inside the workspace
// and returns its persisted fingerprint.
func (o *ProjectMCPFilesOwner) readBack(files []passthroughconfig.File) ([]types.ProjectMCPPreparedFile, error) {
	if len(files) == 0 {
		return nil, nil
	}
	root, err := os.OpenRoot(o.manager.cfg.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("open project MCP workspace root: %w", err)
	}
	defer func() { _ = root.Close() }()
	evidence := make([]types.ProjectMCPPreparedFile, 0, len(files))
	for _, file := range files {
		relative, err := projectMCPRelativePath(o.manager.cfg.WorkDir, file.Path)
		if err != nil {
			return nil, err
		}
		info, err := root.Lstat(relative)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("project MCP file %q was not materialized", file.Path)
		}
		content, err := root.ReadFile(relative)
		if err != nil || len(content) == 0 {
			return nil, fmt.Errorf("project MCP file %q is unreadable", file.Path)
		}
		evidence = append(evidence, types.ProjectMCPPreparedFile{
			Path: file.Path, Fingerprint: passthroughconfig.Fingerprint(content),
		})
	}
	return evidence, nil
}

func projectMCPRelativePath(workspace, target string) (string, error) {
	relative, err := filepath.Rel(workspace, target)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) {
		return "", errors.New("project MCP file is outside the helper workspace")
	}
	for _, component := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		if component == ".." {
			return "", errors.New("project MCP file escapes the helper workspace")
		}
	}
	return relative, nil
}
