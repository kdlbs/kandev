package process

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/imagepreview"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-QUESTION-IMAGES-002.6
func TestQuestionImageSelfRepositoryScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "actual-repository")
	require.NoError(t, os.Mkdir(root, 0o755))
	initGitRepoAt(t, root)
	want := writeQuestionImageFixture(t, root, 2)

	manager := NewManager(&config.InstanceConfig{WorkDir: root}, logger.Default())
	t.Cleanup(manager.stopWorkspaceTrackers)

	for _, test := range []struct {
		name string
		repo string
	}{
		{name: "omitted repository scope"},
		{name: "workspace folder compatibility alias", repo: filepath.Base(root)},
	} {
		t.Run(test.name, func(t *testing.T) {
			content, err := manager.ReadQuestionImage(t.Context(), "screen.png", test.repo)
			require.NoError(t, err)
			require.Equal(t, want, content.Bytes)
		})
	}

	t.Run("relative dot workspace folder compatibility alias", func(t *testing.T) {
		relativeRoot := filepath.Join(t.TempDir(), "relative-repository")
		require.NoError(t, os.Mkdir(relativeRoot, 0o755))
		initGitRepoAt(t, relativeRoot)
		want := writeQuestionImageFixture(t, relativeRoot, 2)
		t.Chdir(relativeRoot)

		manager := NewManager(&config.InstanceConfig{WorkDir: "."}, logger.Default())
		t.Cleanup(manager.stopWorkspaceTrackers)
		content, err := manager.ReadQuestionImage(t.Context(), "screen.png", filepath.Base(relativeRoot))
		require.NoError(t, err)
		require.Equal(t, want, content.Bytes)
	})

	t.Run("configured workspace name stays visible through symlink", func(t *testing.T) {
		actualRoot := filepath.Join(t.TempDir(), "actual-repository")
		require.NoError(t, os.Mkdir(actualRoot, 0o755))
		initGitRepoAt(t, actualRoot)
		want := writeQuestionImageFixture(t, actualRoot, 2)
		visibleRoot := filepath.Join(t.TempDir(), "agent-visible-workspace")
		symlinkOrSkip(t, actualRoot, visibleRoot)

		manager := NewManager(&config.InstanceConfig{WorkDir: visibleRoot}, logger.Default())
		t.Cleanup(manager.stopWorkspaceTrackers)
		content, err := manager.ReadQuestionImage(t.Context(), "screen.png", filepath.Base(visibleRoot))
		require.NoError(t, err)
		require.Equal(t, want, content.Bytes)
		_, err = manager.ReadQuestionImage(t.Context(), "screen.png", filepath.Base(actualRoot))
		require.ErrorIs(t, err, imagepreview.NotFound)
	})
}

// @covers AC-TASKS-QUESTION-IMAGES-002.3, AC-TASKS-QUESTION-IMAGES-002.6
func TestQuestionImageRepositoryScopePrecedence(t *testing.T) {
	t.Run("existing child wins", func(t *testing.T) {
		root, manager := newQuestionImageRepositoryFixture(t)
		rootImage := writeQuestionImageFixture(t, root, 2)
		child := filepath.Join(root, filepath.Base(root))
		require.NoError(t, os.Mkdir(child, 0o755))
		childImage := writeQuestionImageFixture(t, child, 3)
		content, err := manager.ReadQuestionImage(t.Context(), "screen.png", filepath.Base(root))
		require.NoError(t, err)
		require.NotEqual(t, rootImage, childImage)
		require.Equal(t, childImage, content.Bytes)
	})

	t.Run("missing child image does not fall back", func(t *testing.T) {
		root, manager := newQuestionImageRepositoryFixture(t)
		_ = writeQuestionImageFixture(t, root, 2)
		require.NoError(t, os.Mkdir(filepath.Join(root, filepath.Base(root)), 0o755))
		content, err := manager.ReadQuestionImage(t.Context(), "screen.png", filepath.Base(root))
		require.Nil(t, content)
		require.ErrorIs(t, err, imagepreview.NotFound)
	})

	t.Run("broken child symlink does not fall back", func(t *testing.T) {
		root, manager := newQuestionImageRepositoryFixture(t)
		_ = writeQuestionImageFixture(t, root, 2)
		child := filepath.Join(root, filepath.Base(root))
		symlinkOrSkip(t, filepath.Join(t.TempDir(), "missing"), child)
		content, err := manager.ReadQuestionImage(t.Context(), "screen.png", filepath.Base(root))
		require.Nil(t, content)
		require.ErrorIs(t, err, imagepreview.NotFound)
	})

	t.Run("escaping child symlink does not fall back", func(t *testing.T) {
		root, manager := newQuestionImageRepositoryFixture(t)
		_ = writeQuestionImageFixture(t, root, 2)
		outside := t.TempDir()
		_ = writeQuestionImageFixture(t, outside, 3)
		symlinkOrSkip(t, outside, filepath.Join(root, filepath.Base(root)))
		content, err := manager.ReadQuestionImage(t.Context(), "screen.png", filepath.Base(root))
		require.Nil(t, content)
		require.Error(t, err)
	})
}

// @covers AC-TASKS-QUESTION-IMAGES-002.6
func TestQuestionImageRepositoryScopeAliasRequiresOwnRepository(t *testing.T) {
	t.Run("unknown scope stays unknown", func(t *testing.T) {
		root, manager := newQuestionImageRepositoryFixture(t)
		_ = writeQuestionImageFixture(t, root, 2)
		content, err := manager.ReadQuestionImage(t.Context(), "screen.png", "unknown-child")
		require.Nil(t, content)
		require.ErrorIs(t, err, imagepreview.NotFound)
	})

	t.Run("bare workspace does not gain alias", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bare-workspace")
		require.NoError(t, os.Mkdir(root, 0o755))
		childRepository := filepath.Join(root, "child-repository")
		require.NoError(t, os.Mkdir(childRepository, 0o755))
		initGitRepoAt(t, childRepository)
		_ = writeQuestionImageFixture(t, root, 2)
		manager := NewManager(&config.InstanceConfig{WorkDir: root}, logger.Default())
		t.Cleanup(manager.stopWorkspaceTrackers)
		content, err := manager.ReadQuestionImage(t.Context(), "screen.png", filepath.Base(root))
		require.Nil(t, content)
		require.ErrorIs(t, err, imagepreview.NotFound)
	})
}

func newQuestionImageRepositoryFixture(t *testing.T) (string, *Manager) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, os.Mkdir(root, 0o755))
	initGitRepoAt(t, root)
	manager := NewManager(&config.InstanceConfig{WorkDir: root}, logger.Default())
	t.Cleanup(manager.stopWorkspaceTrackers)
	return root, manager
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
}
