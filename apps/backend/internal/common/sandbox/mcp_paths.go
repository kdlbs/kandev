package sandbox

import (
	"slices"
	"strings"
)

const MCPRootPath = "/mcp"
const MCPRoutesCapability = "sandbox-mcp-routes.v1"
const MCPPackagePathPrefix = "/mcp/packages/"

// CanonicalMCPPaths validates an exact route set. Legacy captures keep core only.
func CanonicalMCPPaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return []string{MCPRootPath}, nil
	}
	if len(paths) > 32 || !slices.Contains(paths, MCPRootPath) {
		return nil, ErrInvalidPolicy
	}
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if !validMCPPath(path) || seen[path] {
			return nil, ErrInvalidPolicy
		}
		seen[path] = true
	}
	result := slices.Clone(paths)
	slices.Sort(result)
	return result, nil
}

func validMCPPath(path string) bool {
	if path == MCPRootPath {
		return true
	}
	name, found := strings.CutPrefix(path, MCPPackagePathPrefix)
	if !found || name == "" || len(name) > 64 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	return strings.IndexFunc(name, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-'
	}) == -1
}

// MCPURLForPath preserves the private endpoint and admits only captured routes.
func (l *Launch) MCPURLForPath(path string) (string, error) {
	if l == nil || !validMCPPath(path) || !validMCPRoute(l.MCPURL) {
		return "", ErrInvalidPolicy
	}
	paths, err := CanonicalMCPPaths(l.MCPPaths)
	if err != nil || !slices.Contains(paths, path) {
		return "", ErrInvalidPolicy
	}
	if l.MCPURL == "" {
		return "", nil
	}
	return l.MCPURL + strings.TrimPrefix(path, MCPRootPath), nil
}

func (l *Launch) validateMCP() error {
	if !validMCPRoute(l.MCPURL) {
		return ErrInvalidPolicy
	}
	if len(l.MCPPaths) == 0 {
		return nil
	}
	if _, err := CanonicalMCPPaths(l.MCPPaths); err != nil || l.MCPURL == "" {
		return ErrInvalidPolicy
	}
	return nil
}
