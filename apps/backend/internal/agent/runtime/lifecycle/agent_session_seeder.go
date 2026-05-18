package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/remoteauth"
	"github.com/kandev/kandev/internal/common/logger"
)

// agentSessionsRootSubdir is the subdirectory under the kandev home where
// per-container agent session dirs live. e.g. ~/.kandev/agent-sessions/.
const agentSessionsRootSubdir = "agent-sessions"

// authMethodTypeFiles selects RemoteAuthMethod entries that copy local files
// into the remote environment (vs env-var-based auth).
const authMethodTypeFiles = "files"

// AgentSessionsRoot returns the directory under the kandev home where
// per-container session dirs are created (e.g. ~/.kandev/agent-sessions/).
func AgentSessionsRoot(kandevHomeDir string) string {
	if kandevHomeDir == "" {
		return ""
	}
	return filepath.Join(kandevHomeDir, agentSessionsRootSubdir)
}

// InstanceSessionRoot returns the per-container parent dir that mimics the
// agent's home (e.g. ~/.kandev/agent-sessions/<instance_id>/). Files seeded
// from the host land under this path keyed by their RemoteAuth.TargetRelDir,
// matching the tree the bind-mount target expects inside the container.
func InstanceSessionRoot(kandevHomeDir, instanceID string) string {
	if kandevHomeDir == "" || instanceID == "" {
		return ""
	}
	return filepath.Join(AgentSessionsRoot(kandevHomeDir), instanceID)
}

// SessionDirHostPath returns the kandev-managed bind-mount source for one
// agent in one container. It mirrors the structure SessionDirTarget expects
// inside the container: e.g. SessionDirTemplate "{home}/.codex" + target
// "/root/.codex" → host path "<kandev-home>/agent-sessions/<instanceID>/
// .codex". The leaf dotdir is preserved so the tree on the host matches the
// tree the agent expects, which means UploadCredentialFiles + the
// inside-container view of the bind mount are byte-identical.
func SessionDirHostPath(kandevHomeDir, instanceID, sessionDirTemplate string) string {
	if sessionDirTemplate == "" {
		return ""
	}
	root := InstanceSessionRoot(kandevHomeDir, instanceID)
	if root == "" {
		return ""
	}
	rel := strings.TrimPrefix(sessionDirTemplate, "{home}/")
	return filepath.Join(root, rel)
}

// localFileUploader implements FileUploader for the host filesystem. Used by
// the docker session seeder to copy auth files from ~/<src> into the
// kandev-managed per-container session dir. The uploader is bound to a
// session root and refuses any write that resolves outside it: instanceID
// flows into that root from internal callers, but the containment check
// keeps a malformed input from escaping into ~/.ssh, /etc, etc.
type localFileUploader struct {
	root string
}

func (u localFileUploader) WriteFile(_ context.Context, path string, data []byte, mode os.FileMode) error {
	cleanPath, err := containedPath(u.root, path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(cleanPath), err)
	}
	return os.WriteFile(cleanPath, data, mode)
}

// containedPath returns a cleaned absolute path guaranteed to be inside root,
// or an error if the path escapes via traversal or absolute-path injection.
// Used as the sanitiser between caller-supplied paths and host writes.
func containedPath(root, path string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("session root is empty")
	}
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return "", fmt.Errorf("path %q outside session root %q: %w", path, root, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes session root %q", path, root)
	}
	return cleanPath, nil
}

// SeedAgentSessionDir copies the agent's RemoteAuth.SourceFiles from the host
// home into a per-container session dir, populating just the auth-relevant
// files (typically auth.json + config.toml). Stale per-host state — codex's
// state.db with absolute macOS paths, sessions/ subdirectories, lock files —
// is intentionally NOT copied; the agent rebuilds them inside the container
// with container-local Linux paths.
//
// instanceSessionRoot is the container's session root (e.g. <kandev-home>/
// agent-sessions/<container_id>). The function selects only "files"-type
// remote-auth methods that match the host OS.
func SeedAgentSessionDir(
	ctx context.Context,
	ag agents.Agent,
	instanceSessionRoot string,
	log *logger.Logger,
) error {
	if ag == nil || instanceSessionRoot == "" {
		return nil
	}
	// Always materialise the per-instance root so the container manager has a
	// stable bind-mount source even for agents whose auth is env-only (claude,
	// gemini, …) and the dotdir is otherwise empty until the in-container
	// SetupScript fills it.
	if err := os.MkdirAll(instanceSessionRoot, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", instanceSessionRoot, err)
	}

	auth := ag.RemoteAuth()
	if auth == nil {
		return nil
	}
	hostOS := runtime.GOOS
	methods := make([]remoteauth.Method, 0, len(auth.Methods))
	for _, m := range auth.Methods {
		if m.Type != authMethodTypeFiles || len(m.SourceFiles) == 0 {
			continue
		}
		// Resolve the OS-specific source list into a flat one keyed by host
		// OS, falling back to "linux" for cross-platform agents.
		srcs := selectAuthSourceFiles(m, hostOS)
		if len(srcs) == 0 {
			continue
		}
		methods = append(methods, remoteauth.Method{
			Type:         m.Type,
			SourceFiles:  srcs,
			TargetRelDir: m.TargetRelDir,
		})
	}
	if len(methods) == 0 {
		return nil
	}

	return UploadCredentialFiles(ctx, localFileUploader{root: instanceSessionRoot}, methods, instanceSessionRoot, log)
}

// selectAuthSourceFiles picks the SourceFiles list to use for the host OS,
// degrading gracefully to whatever the agent ships for "linux" or generic if
// the host-specific entry is absent. The returned slice is the relative paths
// (e.g. ".codex/auth.json") expected by UploadCredentialFiles.
func selectAuthSourceFiles(m agents.RemoteAuthMethod, hostOS string) []string {
	if v, ok := m.SourceFiles[hostOS]; ok {
		return v
	}
	if v, ok := m.SourceFiles["linux"]; ok {
		return v
	}
	if v, ok := m.SourceFiles[""]; ok {
		return v
	}
	return nil
}

// CleanupAgentSessionDir removes the per-container session root from disk.
// Best-effort: a missing directory or a transient I/O error is logged at
// debug level and not propagated, since cleanup runs after teardown. Called
// only on destructive stops (task/session deleted/archived).
func CleanupAgentSessionDir(instanceSessionRoot string, log *logger.Logger) {
	if instanceSessionRoot == "" {
		return
	}
	if err := os.RemoveAll(instanceSessionRoot); err != nil {
		log.Debug("failed to remove per-container agent session dir",
			zap.String("path", instanceSessionRoot), zap.Error(err))
	}
}
