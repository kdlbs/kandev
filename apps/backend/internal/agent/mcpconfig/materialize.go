package mcpconfig

import (
	"fmt"
	"strings"
)

// ManagedPackageCommand returns the executor-side stdio command for a pinned
// npm package. The package/version is always exact; the executor owns the
// package cache used by its Node runtime.
func ManagedPackageCommand(configuration MCPServerConfiguration) (string, []string, error) {
	if configuration.PackageType != "npm" || strings.TrimSpace(configuration.PackageName) == "" || !exactPackageVersion(configuration.PackageVersion) {
		return "", nil, fmt.Errorf("%w: managed packages require an npm name and exact version", ErrMCPInvalidDefinition)
	}
	packageSpec := configuration.PackageName + "@" + configuration.PackageVersion
	return "npx", []string{"--yes", "--package", packageSpec, configuration.PackageName}, nil
}
