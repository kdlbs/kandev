package mcpconfig

import (
	"fmt"
	"path"
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
	executable := strings.TrimSpace(configuration.PackageExecutable)
	if executable == "" && configuration.Options != nil {
		if hint, ok := configuration.Options["runtime_hint"].(string); ok {
			executable = strings.TrimSpace(hint)
		}
	}
	if executable == "" {
		executable = path.Base(configuration.PackageName)
	}
	args := []string{"--yes", "--package", packageSpec}
	if registry := strings.TrimSpace(configuration.PackageRegistry); registry != "" {
		args = append(args, "--registry", registry)
	}
	args = append(args, configuration.PackageRuntimeArguments...)
	args = append(args, executable)
	args = append(args, configuration.PackageArguments...)
	return "npx", args, nil
}
