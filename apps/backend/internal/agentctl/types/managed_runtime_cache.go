package types

const (
	ManagedRuntimePrivatePreparationCapability = "managed_runtime_private_preparation_v1"
	ManagedRuntimeCacheRootManifestMissing     = "root_manifest_missing"
)

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
}
