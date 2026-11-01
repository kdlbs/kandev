package main

import (
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

// loadNodeLaunchWrapperFile is replaced only by tests of the flag plumbing.
var loadNodeLaunchWrapperFile = launchwrapper.LoadNodeFile

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
	spec, err := loadNodeLaunchWrapperFile(path, ownerUID)
	if err != nil {
		return err
	}
	cfg.NodeLaunchWrapper = launchwrapper.Node{Spec: spec, Required: required}
	return nil
}

// applyNodeAllocDir hands the node's allocation directory to the loaded
// wrapper. A policy that references it fails startup without a usable value,
// so a misconfigured node refuses to start instead of failing every launch.
// Launches resolve and check the value again.
func applyNodeAllocDir(cfg *config.Config, allocDir string) error {
	node := cfg.NodeLaunchWrapper
	if node.Spec.IsZero() {
		if allocDir != "" {
			return errors.New("--node-launch-wrapper-alloc-dir needs --node-launch-wrapper-file")
		}
		return nil
	}
	if launchwrapper.UsesAllocDir(node.Spec.Argv) {
		if _, err := launchwrapper.ResolveAllocDir(allocDir); err != nil {
			return fmt.Errorf("the node launch wrapper references the allocation directory, which --node-launch-wrapper-alloc-dir must supply: %w", err)
		}
	}
	cfg.NodeLaunchWrapper.AllocDir = allocDir
	return nil
}
