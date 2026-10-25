// Package sandbox contains the provider-neutral confinement contract.
package sandbox

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"
)

const ProtocolHTTPS = "https"

var (
	ErrInvalidPolicy  = errors.New("sandbox policy is invalid")
	ErrOutsideCeiling = errors.New("sandbox policy exceeds the executor ceiling")
)

type FilesystemGrant struct {
	Path      string `json:"path,omitempty"`
	Reference string `json:"reference,omitempty"`
	Writable  bool   `json:"writable"`
}

type NetworkGrant struct {
	Host     string `json:"host"`
	Port     uint16 `json:"port"`
	Protocol string `json:"protocol"`
}

type Grants struct {
	Filesystem      []FilesystemGrant `json:"filesystem"`
	Network         []NetworkGrant    `json:"network"`
	KandevMCP       bool              `json:"kandev_mcp"`
	EnvironmentKeys []string          `json:"environment_keys,omitempty"`
}

// Canonicalize resolves paths on the execution host before admission.
func Canonicalize(grants Grants, protectedRoots []string) (Grants, error) {
	if len(grants.Filesystem) > 256 || len(grants.Network) > 256 {
		return Grants{}, ErrInvalidPolicy
	}
	protected, err := canonicalProtectedRoots(protectedRoots)
	if err != nil {
		return Grants{}, err
	}
	filesystem, err := canonicalFilesystem(grants.Filesystem, protected)
	if err != nil {
		return Grants{}, err
	}
	keys, err := canonicalEnvironmentKeys(grants.EnvironmentKeys)
	if err != nil {
		return Grants{}, err
	}
	out := Grants{KandevMCP: grants.KandevMCP, Filesystem: filesystem, Network: make([]NetworkGrant, 0, len(grants.Network))}
	out.EnvironmentKeys = keys
	routes := make(map[NetworkGrant]bool)
	for _, grant := range grants.Network {
		grant.Host = strings.ToLower(grant.Host)
		if !validRoute(grant) {
			return Grants{}, ErrInvalidPolicy
		}
		if !routes[grant] {
			out.Network = append(out.Network, grant)
			routes[grant] = true
		}
	}
	slices.SortFunc(out.Network, compareRoutes)
	return out, nil
}

func canonicalProtectedRoots(roots []string) ([]string, error) {
	if runtime.GOOS == "linux" {
		roots = append(slices.Clone(roots), "/sys/fs/cgroup")
	}
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		if !filepath.IsAbs(root) || strings.ContainsFunc(root, unicode.IsControl) {
			return nil, ErrInvalidPolicy
		}
		current := filepath.Clean(root)
		for {
			resolved, err := filepath.EvalSymlinks(current)
			if err == nil {
				suffix, err := filepath.Rel(current, root)
				if err != nil {
					return nil, ErrInvalidPolicy
				}
				out = append(out, filepath.Join(resolved, suffix))
				break
			}
			parent := filepath.Dir(current)
			if !errors.Is(err, os.ErrNotExist) || parent == current {
				return nil, ErrInvalidPolicy
			}
			current = parent
		}
	}
	return out, nil
}

// WithinCeiling refuses grants that are not covered by the executor ceiling.
func WithinCeiling(grants, ceiling Grants) error {
	for _, key := range grants.EnvironmentKeys {
		if !slices.Contains(ceiling.EnvironmentKeys, key) {
			return ErrOutsideCeiling
		}
	}
	if grants.KandevMCP && !ceiling.KandevMCP {
		return ErrOutsideCeiling
	}
	for _, grant := range grants.Filesystem {
		if !slices.ContainsFunc(ceiling.Filesystem, func(limit FilesystemGrant) bool {
			return filesystemCovered(grant, limit) && (!grant.Writable || limit.Writable)
		}) {
			return ErrOutsideCeiling
		}
	}
	for _, grant := range grants.Network {
		if !slices.Contains(ceiling.Network, grant) {
			return ErrOutsideCeiling
		}
	}
	return nil
}

func within(path, root string) bool {
	if !filepath.IsAbs(path) || !filepath.IsAbs(root) {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func canonicalPath(path string) (string, error) {
	if len(path) > 4096 || !filepath.IsAbs(path) || strings.ContainsFunc(path, unicode.IsControl) {
		return "", ErrInvalidPolicy
	}
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil || resolved != clean {
		return "", ErrInvalidPolicy
	}
	return clean, nil
}

func validRoute(grant NetworkGrant) bool {
	if grant.Port == 0 || (grant.Protocol != ProtocolHTTPS && grant.Protocol != "http" && grant.Protocol != "tcp") {
		return false
	}
	if addr, err := netip.ParseAddr(grant.Host); err == nil {
		return !addr.IsUnspecified() && !addr.IsMulticast() && addr.Zone() == ""
	}
	return validHostname(grant.Host)
}

func validHostname(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

func compareRoutes(a, b NetworkGrant) int {
	if n := strings.Compare(a.Host, b.Host); n != 0 {
		return n
	}
	if a.Port < b.Port {
		return -1
	}
	if a.Port > b.Port {
		return 1
	}
	return strings.Compare(a.Protocol, b.Protocol)
}
