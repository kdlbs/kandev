package types

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

const (
	// ProjectMCPDeliveryVersion versions execution-bound project delivery
	// evidence on the authenticated configure contract.
	ProjectMCPDeliveryVersion = 1

	// ProjectMCPDeliveryCapability is advertised by an agentctl helper that can
	// accept project delivery evidence. A helper without it must not be asked
	// to configure a package-mode launch that depends on project delivery.
	ProjectMCPDeliveryCapability = "project-mcp-delivery-v1"

	// ProjectMCPDeliveryAgentPi is the only agent that consumes the Pi project
	// file. Strategy evidence for another agent must be rejected.
	ProjectMCPDeliveryAgentPi = "pi-acp"

	// ProjectMCPDeliveryStrategyPi is the only project-file strategy admitted
	// to produce delivery evidence today. Declaring a project strategy is not
	// enough; each producer needs explicit admission support.
	ProjectMCPDeliveryStrategyPi = "pi"

	projectMCPDeliveryMaxIdentityBytes  = 256
	projectMCPDeliveryMaxWorkspaceBytes = 4096
	projectMCPDeliveryDigestHexLength   = 64
	projectMCPDeliveryMaxPackages       = 16
)

// ProjectMCPDelivery is execution-bound proof that the host materialized every
// selected built-in package into a project configuration file for one agent
// process. It carries no endpoint, credential, header, environment value, or
// file content. Only the trusted backend launch builder can supply it.
type ProjectMCPDelivery struct {
	Version       int      `json:"version"`
	Strategy      string   `json:"strategy"`
	Workspace     string   `json:"workspace"`
	TaskID        string   `json:"task_id"`
	InstanceID    string   `json:"instance_id"`
	BindingDigest string   `json:"binding_digest"`
	Packages      []string `json:"packages"`
}

// Clone returns an independent copy so a caller cannot mutate accepted
// configuration state through a shared slice.
func (d *ProjectMCPDelivery) Clone() *ProjectMCPDelivery {
	if d == nil {
		return nil
	}
	clone := *d
	clone.Packages = slices.Clone(d.Packages)
	return &clone
}

// Validate rejects malformed or widened delivery evidence before it can
// authorize a session operation.
func (d *ProjectMCPDelivery) Validate() error {
	if d == nil {
		return errors.New("project MCP delivery evidence is missing")
	}
	if d.Version != ProjectMCPDeliveryVersion {
		return fmt.Errorf("unsupported project MCP delivery version %d", d.Version)
	}
	if d.Strategy != ProjectMCPDeliveryStrategyPi {
		return fmt.Errorf("unsupported project MCP delivery strategy %q", d.Strategy)
	}
	for field, value := range map[string]string{
		"task":     d.TaskID,
		"instance": d.InstanceID,
		"binding":  d.BindingDigest,
	} {
		if !validProjectMCPDeliveryIdentity(value) {
			return fmt.Errorf("project MCP delivery %s identity is missing or invalid", field)
		}
	}
	// Workspace paths are not short identifiers: a valid nested workspace can
	// exceed the task/instance bound, so validate it against a path-sized cap.
	if !validProjectMCPDeliveryWorkspace(d.Workspace) {
		return errors.New("project MCP delivery workspace is missing or invalid")
	}
	if len(d.BindingDigest) != projectMCPDeliveryDigestHexLength || d.BindingDigest != strings.ToLower(d.BindingDigest) {
		return errors.New("project MCP delivery binding digest is invalid")
	}
	if _, err := hex.DecodeString(d.BindingDigest); err != nil {
		return errors.New("project MCP delivery binding digest is invalid")
	}
	if len(d.Packages) == 0 || len(d.Packages) > projectMCPDeliveryMaxPackages {
		return errors.New("project MCP delivery names no packages or too many packages")
	}
	seen := make(map[mcpprofile.PackageID]struct{}, len(d.Packages))
	for _, raw := range d.Packages {
		id := mcpprofile.PackageID(raw)
		if !mcpprofile.PackageEligible(id, mcpprofile.SurfaceKanbanTask) {
			return fmt.Errorf("project MCP delivery names unknown package %q", raw)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("project MCP delivery repeats package %q", raw)
		}
		seen[id] = struct{}{}
	}
	return nil
}

// Delivers reports whether this record names the exact package.
func (d *ProjectMCPDelivery) Delivers(pkg mcpprofile.PackageID) bool {
	if d == nil || pkg == "" {
		return false
	}
	return slices.Contains(d.Packages, string(pkg))
}

func validProjectMCPDeliveryIdentity(value string) bool {
	return value != "" &&
		len(value) <= projectMCPDeliveryMaxIdentityBytes &&
		strings.TrimSpace(value) == value &&
		!strings.ContainsRune(value, '\x00')
}

func validProjectMCPDeliveryWorkspace(value string) bool {
	return value != "" &&
		len(value) <= projectMCPDeliveryMaxWorkspaceBytes &&
		strings.TrimSpace(value) == value &&
		!strings.ContainsRune(value, '\x00')
}

// CanonicalProjectMCPDeliveryWorkspace returns a separator-stable workspace
// identity. POSIX-style paths use slash semantics so a Windows backend and a
// Linux helper agree on the same remote workspace; native paths use host
// semantics.
func CanonicalProjectMCPDeliveryWorkspace(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "/") {
		return path.Clean(value)
	}
	return filepath.Clean(value)
}
