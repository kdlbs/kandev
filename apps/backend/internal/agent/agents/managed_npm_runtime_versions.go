package agents

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/agent/managedruntime"
)

//go:embed managed_npm_runtime_versions.json
var managedNPMRuntimeVersionsJSON []byte

var (
	managedNPMRuntimeVersionsOnce sync.Once
	managedNPMRuntimeVersions     map[string]string
	managedNPMRuntimeVersionsErr  error
)

// DefaultManagedNPMRuntimeVersion returns the reviewed exact default for a
// trusted managed npm package.
func DefaultManagedNPMRuntimeVersion(packageName string) (string, error) {
	packageName = strings.TrimSpace(packageName)
	if packageName == "" {
		return "", fmt.Errorf("managed runtime package is empty")
	}
	managedNPMRuntimeVersionsOnce.Do(loadManagedNPMRuntimeVersions)
	if managedNPMRuntimeVersionsErr != nil {
		return "", managedNPMRuntimeVersionsErr
	}
	version, ok := managedNPMRuntimeVersions[packageName]
	if !ok {
		return "", fmt.Errorf("no reviewed default for managed runtime package %q", packageName)
	}
	return version, nil
}

// MustDefaultManagedNPMRuntimeVersion returns a reviewed default or panics.
// Built-in agents call this while the registry is loaded, so a malformed or
// incomplete catalogue fails startup instead of creating an unversioned
// command.
func MustDefaultManagedNPMRuntimeVersion(packageName string) string {
	version, err := DefaultManagedNPMRuntimeVersion(packageName)
	if err != nil {
		panic(fmt.Sprintf("managed runtime defaults are invalid: %v", err))
	}
	return version
}

func loadManagedNPMRuntimeVersions() {
	var versions map[string]string
	if err := json.Unmarshal(managedNPMRuntimeVersionsJSON, &versions); err != nil {
		managedNPMRuntimeVersionsErr = fmt.Errorf("decode managed runtime defaults: %w", err)
		return
	}
	if len(versions) == 0 {
		managedNPMRuntimeVersionsErr = fmt.Errorf("managed runtime defaults are empty")
		return
	}
	for packageName, version := range versions {
		if strings.TrimSpace(packageName) == "" {
			managedNPMRuntimeVersionsErr = fmt.Errorf("managed runtime defaults contain an empty package")
			return
		}
		if _, err := managedruntime.ParseStableVersion(version); err != nil {
			managedNPMRuntimeVersionsErr = fmt.Errorf("managed runtime default %q for %q: %w", version, packageName, err)
			return
		}
	}
	managedNPMRuntimeVersions = versions
}
