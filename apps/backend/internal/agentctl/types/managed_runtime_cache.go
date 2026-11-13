package types

import "strings"

const (
	ManagedRuntimePrivatePreparationCapability = "managed_runtime_private_preparation_v1"
	ManagedRuntimeRepairReuseCapability        = "managed_runtime_repair_reuse_v1"
	ManagedRuntimeCacheRootManifestMissing     = "root_manifest_missing"
)

// ManagedRuntimeInstallContext carries authenticated installer-environment
// provenance for reusable managed npm repairs. It never names a physical path:
// agentctl derives its own store and generation locations locally.
type ManagedRuntimeInstallContext struct {
	InstallerEnvDigest string `json:"installer_env_digest,omitempty"`
	// OwnerScope is the trusted authorization-owner boundary. It is one of
	// task:<owner>, agent-wide:<owner>, or profile:<authorization scope>.
	OwnerScope string `json:"owner_scope,omitempty"`
}

// Valid reports whether the context carries a bounded installer fingerprint.
func (c *ManagedRuntimeInstallContext) Valid() bool {
	if c == nil || c.InstallerEnvDigest == "" || c.OwnerScope == "" {
		return false
	}
	if len(c.InstallerEnvDigest) != 64 || len(c.OwnerScope) > 512 ||
		strings.ContainsAny(c.OwnerScope, "\x00\r\n") {
		return false
	}
	for _, character := range c.InstallerEnvDigest {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

// Clone returns an independent copy.
func (c *ManagedRuntimeInstallContext) Clone() *ManagedRuntimeInstallContext {
	if c == nil {
		return nil
	}
	cloned := *c
	return &cloned
}

// ManagedRuntimeCacheEvidence is sanitized evidence for one failed managed
// runtime startup or probe attempt.
type ManagedRuntimeCacheEvidence struct {
	Condition         string `json:"condition"`
	PackageSpec       string `json:"package_spec"`
	ProcessGeneration uint64 `json:"process_generation,omitempty"`
	ProbeAttemptID    string `json:"probe_attempt_id,omitempty"`
}

// ManagedRuntimePreparationDirective binds private preparation to one
// previously verified failure and its trusted exact package.
type ManagedRuntimePreparationDirective struct {
	Condition               string `json:"condition"`
	PackageSpec             string `json:"package_spec"`
	FailedProcessGeneration uint64 `json:"failed_process_generation,omitempty"`
	ProbeAttemptID          string `json:"probe_attempt_id,omitempty"`
	// RemainingBudgetSeconds is the caller's remaining recovery budget. Zero
	// uses agentctl's local cap; an expired or malformed value rejects the
	// directive. It never extends the caller's own deadline.
	RemainingBudgetSeconds int `json:"remaining_budget_seconds,omitempty"`
	// DeadlineUnixNano is the caller's exact recovery deadline. Agentctl uses
	// it instead of reconstructing a deadline from the seconds field.
	DeadlineUnixNano int64 `json:"deadline_unix_nano,omitempty"`
}
