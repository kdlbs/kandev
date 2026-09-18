package maintenance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantMaintenanceUnavailableSandboxStaysProposal(t *testing.T) {
	sandbox, source, grant := sandboxFixture(t)
	sandbox.run = func(_ context.Context, _, _ string, args ...string) (string, int, error) {
		if slices.Contains(args, "image") {
			return grant.Scope.Image + " linux", 0, nil
		}
		return "", 125, errors.New("container isolation unavailable")
	}
	_, _, err := sandbox.Inspect(context.Background(), source, grant.Scope)
	require.Error(t, err, "an installed image alone does not qualify safe repair execution")
}

func TestAssistantMaintenanceConcurrentFileCAS(t *testing.T) {
	sandbox, source, grant := sandboxFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, sandbox.Prepare(ctx, source, grant, func(context.Context) error { return nil }))
	file, err := sandbox.Read(ctx, grant, "scripts/example.js")
	require.NoError(t, err)
	file.Content = "module.exports = 2;\n"
	ready := make(chan struct{})
	var waiting atomic.Int32
	guard := func(ctx context.Context) error {
		if waiting.Add(1) == 2 {
			close(ready)
		}
		select {
		case <-ready:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := sandbox.Patch(ctx, grant, file, guard); results <- err }()
	}
	first, second := <-results, <-results
	if first == nil {
		require.ErrorIs(t, second, models.ErrConflict)
	} else {
		require.ErrorIs(t, first, models.ErrConflict)
		require.NoError(t, second)
	}
}

func TestAssistantMaintenanceBoundaryChangedFiles(t *testing.T) {
	sandbox, source, grant := sandboxFixture(t)
	ctx := context.Background()
	guard := func(context.Context) error { return nil }
	require.NoError(t, sandbox.Prepare(ctx, source, grant, guard))
	dir, err := sandbox.checkout(grant)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(dir, " scripts"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, " scripts/example.js"), []byte("unexpected"), 0600))
	_, err = stageTree(ctx, dir, grant.Scope.Files)
	require.Error(t, err, "leading spaces in a changed filename cannot hide an out-of-scope file")
}

func TestAssistantMaintenanceQualifiedChecksAndCommit(t *testing.T) {
	image := os.Getenv("KANDEV_TEST_MAINTENANCE_IMAGE")
	if image == "" {
		t.Skip("requires an explicit task-owned local Linux image with Node.js")
	}
	sandbox, source, grant := sandboxFixture(t)
	require.NoError(t, os.Mkdir(filepath.Join(source, "tests"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(source, "tests/positive.js"), []byte(`require('node:assert').equal(require('../scripts/example'),2)`), 0600))
	negative := `const fs=require('node:fs'),assert=require('node:assert'),os=require('node:os');
assert.throws(()=>fs.writeFileSync('/workspace/scripts/example.js','bypass'));
assert.throws(()=>fs.writeFileSync('/etc/maintenance-bypass','bypass'));
assert(!fs.existsSync('/var/run/docker.sock'));
assert(!process.env.MAINTENANCE_CANARY_SECRET);
assert(Object.values(os.networkInterfaces()).flat().every(i=>i.internal));
assert.notEqual(process.getuid(),0);`
	require.NoError(t, os.WriteFile(filepath.Join(source, "tests/negative.js"), []byte(negative), 0600))
	fixtureGit(t, source, "add", ".")
	fixtureGit(t, source, "commit", "-qm", "Add example checks")
	grant.Scope.Image = image
	ctx := context.Background()
	var err error
	grant.Scope, grant.BaseOID, err = sandbox.Inspect(ctx, source, grant.Scope)
	require.NoError(t, err)
	require.Contains(t, grant.Scope.Image, "sha256:")
	guard := func(context.Context) error { return nil }
	require.NoError(t, sandbox.Prepare(ctx, source, grant, guard))
	_, err = sandbox.Commit(ctx, grant, guard)
	require.Error(t, err, "a local commit requires passing checks")
	t.Setenv("MAINTENANCE_CANARY_SECRET", "must-not-reach-container")
	checks, err := sandbox.Check(ctx, grant, guard)
	require.NoError(t, err)
	require.False(t, checks.Passed, "the real positive regression starts red")
	require.NotZero(t, checks.Checks[0].ExitCode)
	require.Zero(t, checks.Checks[1].ExitCode, "negative isolation checks must pass")
	file, err := sandbox.Read(ctx, grant, "scripts/example.js")
	require.NoError(t, err)
	file.Content = "module.exports = 2;\n"
	_, err = sandbox.Patch(ctx, grant, file, guard)
	require.NoError(t, err)
	checks, err = sandbox.Check(ctx, grant, guard)
	require.NoError(t, err)
	require.True(t, checks.Passed)
	artifact, err := sandbox.Commit(ctx, grant, guard)
	require.NoError(t, err)
	require.NotEmpty(t, artifact.CommitOID)
	require.Equal(t, grant.BaseOID, fixtureGit(t, source, "rev-parse", "HEAD"), "source repository never moves")
	replay, err := sandbox.Commit(ctx, grant, guard)
	require.NoError(t, err)
	require.Equal(t, artifact, replay, "the same validated tree has one local commit")
	file, err = sandbox.Read(ctx, grant, "scripts/example.js")
	require.NoError(t, err)
	file.Content = "module.exports = 3;\n"
	_, err = sandbox.Patch(ctx, grant, file, guard)
	require.NoError(t, err)
	_, err = sandbox.Commit(ctx, grant, guard)
	require.Error(t, err, "validation is bound to the actual tree")
}
