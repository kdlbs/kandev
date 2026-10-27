//go:build unix

package launchwrapper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNodePolicyCannotBeRemovedByAnEmptyProfile(t *testing.T) {
	t.Setenv(NodePolicyEnv, filepath.Join(t.TempDir(), "missing-policy.json"))
	_, err := PrepareNode([]string{"/bin/echo", "must-not-start"}, Context{WorkDir: t.TempDir()})
	require.ErrorIs(t, err, ErrNodePolicy)
}

func TestNodePolicyFileIntegrityAndComposition(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	policy := filepath.Join(dir, "policy.json")
	wrapper := filepath.Join(dir, "node-wrapper")
	require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nexec \"$@\"\n"), 0700))
	data, err := json.Marshal(Profile([]string{wrapper, WorkDir, "--"}))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(policy, data, 0600))
	spec, err := loadNodePolicy(policy, os.Getuid())
	require.NoError(t, err)
	args, err := Prepare(spec, []string{"/bin/echo", "a value", ""}, Context{WorkDir: dir, Env: []string{"PATH=/usr/bin:/bin"}})
	require.NoError(t, err)
	require.Equal(t, []string{wrapper, dir, "--", "/bin/echo", "a value", ""}, args)
	require.Equal(t, []string{wrapper, WorkDir, "--"}, spec.Argv)
	require.NoError(t, os.Chmod(policy, 0666))
	_, err = loadNodePolicy(policy, os.Getuid())
	require.ErrorIs(t, err, ErrNodePolicy)
	require.NoError(t, os.Chmod(policy, 0600))
	link := filepath.Join(dir, "policy-link.json")
	require.NoError(t, os.Symlink(policy, link))
	_, err = loadNodePolicy(link, os.Getuid())
	require.ErrorIs(t, err, ErrNodePolicy)
	_, err = loadNodePolicy(policy, os.Getuid()+1)
	require.ErrorIs(t, err, ErrNodePolicy)
	require.NoError(t, os.WriteFile(policy, append(data, []byte(" {}")...), 0600))
	_, err = loadNodePolicy(policy, os.Getuid())
	require.ErrorIs(t, err, ErrNodePolicy)
}

func TestNodePolicyAbsentPreservesArguments(t *testing.T) {
	t.Setenv(NodePolicyEnv, "")
	original := []string{"child", "", "a value"}
	got, err := PrepareNode(original, Context{})
	require.NoError(t, err)
	require.Equal(t, original, got)
	got[0] = "modified"
	require.Equal(t, "child", original[0])
}

func TestNodeAllocationReferenceRequiresWorkerAllocation(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	spec := Profile([]string{"/bin/echo", "{{kandev.alloc_dir}}"})
	t.Setenv("NOMAD_ALLOC_DIR", dir)
	got, err := Prepare(spec, []string{"/bin/echo"}, Context{WorkDir: dir})
	require.NoError(t, err)
	require.Equal(t, dir, got[1])
	t.Setenv("NOMAD_ALLOC_DIR", "")
	_, err = Prepare(spec, []string{"/bin/echo"}, Context{WorkDir: dir})
	require.Error(t, err)
}

func TestRequiredNodePolicyCannotBeOmitted(t *testing.T) {
	t.Setenv(NodePolicyEnv, "")
	require.ErrorIs(t, ValidateNodePolicy(), ErrNodePolicy)
}

func TestNodePolicyRejectsReplaceableAncestor(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	outer := filepath.Join(dir, "replaceable")
	parent := filepath.Join(outer, "admin")
	require.NoError(t, os.MkdirAll(parent, 0700))
	require.NoError(t, os.Chmod(outer, 0777))
	path := filepath.Join(parent, "policy.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,"source":"profile","argv":["/bin/echo"]}`), 0600))
	_, err = loadNodePolicy(path, os.Getuid())
	require.ErrorIs(t, err, ErrNodePolicy)
}
