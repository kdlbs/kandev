package maintenance

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-C", dir}, args...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Example", "GIT_AUTHOR_EMAIL=example@example.invalid", "GIT_COMMITTER_NAME=Example", "GIT_COMMITTER_EMAIL=example@example.invalid"}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func sandboxFixture(t *testing.T) (*Sandbox, string, models.MaintenanceGrant) {
	t.Helper()
	source := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(source, "scripts"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(source, "scripts/example.js"), []byte("module.exports = 1;\n"), 0600))
	require.NoError(t, os.Symlink("/tmp/never-read-this", filepath.Join(source, "scripts/link.js")))
	fixtureGit(t, source, "init", "-q")
	fixtureGit(t, source, "add", ".")
	fixtureGit(t, source, "commit", "-qm", "Initial example")
	grant := models.MaintenanceGrant{CandidateID: uuid.NewString(), BindingID: "binding", OwnerUserID: "owner", Revision: 1, BaseOID: fixtureGit(t, source, "rev-parse", "HEAD"), Scope: models.MaintenanceScope{RepositoryID: "repo", WorkflowID: "workflow", WorkflowStepID: "review", ProfileID: "profile", Image: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Files: []string{"scripts/example.js", "scripts/link.js"}, Actions: []string{"read", "patch", "test", "commit"}, Positive: []string{"node", "tests/positive.js"}, Negative: []string{"node", "tests/negative.js"}}}
	return New(filepath.Join(t.TempDir(), "maintenance")), source, grant
}

func TestAssistantMaintenanceIsolatedFiles(t *testing.T) {
	sandbox, source, grant := sandboxFixture(t)
	ctx := context.Background()
	guard := func(context.Context) error { return nil }
	require.NoError(t, sandbox.Prepare(ctx, source, grant, guard))
	file, err := sandbox.Read(ctx, grant, "scripts/example.js")
	require.NoError(t, err)
	require.Equal(t, "module.exports = 1;\n", file.Content)
	_, err = sandbox.Read(ctx, grant, ".git/config")
	require.Error(t, err)
	_, err = sandbox.Read(ctx, grant, "scripts/link.js")
	require.Error(t, err, "symlinks cannot expose another path")
	file.Content = "module.exports = 2;\n"
	_, err = sandbox.Patch(ctx, grant, file, guard)
	require.NoError(t, err)
	_, err = sandbox.Patch(ctx, grant, file, guard)
	require.Error(t, err, "old file hash cannot overwrite a newer edit")
	original, err := os.ReadFile(filepath.Join(source, "scripts/example.js"))
	require.NoError(t, err)
	require.Equal(t, "module.exports = 1;\n", string(original))
	file, err = sandbox.Read(ctx, grant, "scripts/example.js")
	require.NoError(t, err)
	file.Content = "module.exports = 3;\n"
	_, err = sandbox.Patch(ctx, grant, file, func(context.Context) error { return models.ErrConflict })
	require.ErrorIs(t, err, models.ErrConflict)
	actual, err := sandbox.Read(ctx, grant, "scripts/example.js")
	require.NoError(t, err)
	require.Equal(t, "module.exports = 2;\n", actual.Content)
	wrongOwner := grant
	wrongOwner.OwnerUserID = "foreign"
	_, err = sandbox.Read(ctx, wrongOwner, "scripts/example.js")
	require.Error(t, err)
}
