package launchwrapper

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
)

// NodePolicyEnv names the administrator policy inherited by worker agentctl processes.
const NodePolicyEnv = "KANDEV_NODE_LAUNCH_WRAPPER_FILE"

var ErrNodePolicy = errors.New("required node launch policy is unavailable or untrusted")

// PrepareNode applies host policy outside the optional per-profile wrapper. It reads
// agentctl's own environment, never the profile-controlled child environment.
func PrepareNode(inner []string, ctx Context) ([]string, error) {
	path := os.Getenv(NodePolicyEnv)
	if path == "" {
		return slices.Clone(inner), nil
	}
	spec, err := loadNodePolicy(path, 0)
	if err != nil {
		return nil, err
	}
	args, err := Prepare(spec, inner, ctx)
	if err != nil {
		return nil, ErrNodePolicy
	}
	return args, nil
}

// ValidateNodePolicy refuses a required-policy startup without a trusted node policy.
func ValidateNodePolicy() error {
	if os.Getenv(NodePolicyEnv) == "" {
		return ErrNodePolicy
	}
	_, err := loadNodePolicy(os.Getenv(NodePolicyEnv), 0)
	return err
}

func loadNodePolicy(path string, owner int) (Spec, error) {
	var spec Spec
	file, err := openNodePolicy(path, owner)
	if err != nil {
		return spec, err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil || spec.Validate() != nil || !filepath.IsAbs(spec.Argv[0]) {
		return Spec{}, ErrNodePolicy
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return Spec{}, ErrNodePolicy
	}
	return spec, nil
}

func trustedNodePolicyParents(path string, owner int) bool {
	for {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || (!nodePolicyOwned(info, owner) && !nodePolicyOwned(info, 0)) {
			return false
		}
		rootSticky := nodePolicyOwned(info, 0) && info.Mode()&os.ModeSticky != 0
		if info.Mode().Perm()&0022 != 0 && !rootSticky {
			return false
		}
		parent := filepath.Dir(path)
		if parent == path {
			return true
		}
		path = parent
	}
}

func openNodePolicy(path string, owner int) (*os.File, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !filepath.IsAbs(path) || resolved != path {
		return nil, ErrNodePolicy
	}
	if !trustedNodePolicyParents(filepath.Dir(path), owner) {
		return nil, ErrNodePolicy
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || !nodePolicyOwned(before, owner) || before.Mode().Perm()&0022 != 0 || before.Size() > 65536 {
		return nil, ErrNodePolicy
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrNodePolicy
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, ErrNodePolicy
	}
	return file, nil
}
