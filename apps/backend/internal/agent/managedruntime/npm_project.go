package managedruntime

import "github.com/kandev/kandev/internal/common/managednpm"

// NPMProjectPrefix is the trusted execution-host project marker.
const NPMProjectPrefix = managednpm.NPMProjectPrefix

// NPMProjectPrefixArgs returns the canonical managed npm project arguments.
func NPMProjectPrefixArgs() []string { return managednpm.NPMProjectPrefixArgs() }

// PrepareNPMProjectPrefix resolves the trusted project marker on the execution host.
func PrepareNPMProjectPrefix(args []string) error { return managednpm.PrepareNPMProjectPrefix(args) }

// PrepareNPMProjectDirectory returns the private execution-host project directory.
func PrepareNPMProjectDirectory() (string, error) { return managednpm.PrepareNPMProjectDirectory() }
