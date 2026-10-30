package main

import (
	"errors"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

// applyNodeLaunchWrapper loads the operator-owned wrapper. It never falls back
// to an unwrapped launch: a required wrapper without a file, or a named file
// that cannot be trusted, is an error.
func applyNodeLaunchWrapper(cfg *config.Config, path string, required bool, ownerUID int) error {
	if path == "" {
		if required {
			return errors.New("--require-node-launch-wrapper needs --node-launch-wrapper-file")
		}
		return nil
	}
	spec, err := launchwrapper.LoadNodeFile(path, ownerUID)
	if err != nil {
		return err
	}
	cfg.NodeLaunchWrapper = launchwrapper.Node{Spec: spec, Required: required}
	return nil
}
