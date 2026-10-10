package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type turnCheckpointDiffView struct {
	operator    *GitOperator
	environment map[string]string
}

// An empty worktree makes Git read attributes from the private endpoint index.
// The isolated Git directory excludes mutable repository and user overrides.
// Objects are read in place; neither checkout files nor objects are copied.
func (g *GitOperator) newTurnCheckpointDiffView(ctx context.Context, treeOID, hashAlgorithm string) (*turnCheckpointDiffView, func(), error) {
	objects, err := g.turnCheckpointOutput(ctx, "", "rev-parse", "--git-path", "objects")
	if err != nil {
		return nil, nil, err
	}
	objectDirectory := strings.TrimSpace(string(objects))
	if !filepath.IsAbs(objectDirectory) {
		objectDirectory = filepath.Join(g.workDir, objectDirectory)
	}
	objectDirectory, err = filepath.Abs(objectDirectory)
	if err != nil {
		return nil, nil, err
	}
	root, err := os.MkdirTemp("", "kandev-turn-diff-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	view := &turnCheckpointDiffView{
		operator: NewGitOperator(root, g.logger, nil),
		environment: map[string]string{
			"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull,
			"GIT_CONFIG_PARAMETERS": "", "GIT_CONFIG_COUNT": "1",
			"GIT_CONFIG_KEY_0": "core.attributesFile", "GIT_CONFIG_VALUE_0": os.DevNull,
			"GIT_ATTR_NOSYSTEM": "1", "GIT_ATTR_SOURCE": treeOID,
			"GIT_NO_REPLACE_OBJECTS": "1", "GIT_DEFAULT_HASH": hashAlgorithm,
			"GIT_TEMPLATE_DIR": filepath.Join(root, "templates"),
		},
	}
	view.operator.setEnvironmentProvider(g.environment)
	if err := os.Mkdir(view.environment["GIT_TEMPLATE_DIR"], 0o700); err != nil {
		cleanup()
		return nil, nil, err
	}
	if _, err := view.output(ctx, "init", "-q"); err != nil {
		cleanup()
		return nil, nil, err
	}
	view.environment["GIT_OBJECT_DIRECTORY"] = objectDirectory
	if _, err := view.output(ctx, "read-tree", treeOID); err != nil {
		cleanup()
		return nil, nil, err
	}
	return view, cleanup, nil
}

func (v *turnCheckpointDiffView) output(ctx context.Context, args ...string) ([]byte, error) {
	return v.operator.turnCheckpointOutputWithEnvironment(ctx, "", v.environment, args...)
}
