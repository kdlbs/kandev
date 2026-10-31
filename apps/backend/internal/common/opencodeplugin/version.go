// Package opencodeplugin owns compatibility values shared by the backend
// lifecycle and the agentctl-side OpenCode plugin host.
package opencodeplugin

// SupportedRuntimeVersion is the only OpenCode version proved by the packaged
// conformance harness. Additional versions join this manifest only after their
// own recorded proof.
const SupportedRuntimeVersion = "2.2.0"
