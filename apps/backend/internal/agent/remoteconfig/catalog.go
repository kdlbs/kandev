// Package remoteconfig owns the backend allowlist for small agent
// configuration files that may be copied into isolated executors.
package remoteconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/kandev/kandev/internal/agent/agents"
)

// File is safe metadata for one declared configuration file. It contains no
// file contents and never contains an absolute host path.
type File struct {
	SourcePath string `json:"source_path"`
	TargetPath string `json:"target_path"`
	Available  bool   `json:"available"`
}

// Bundle is one stable, selectable configuration bundle.
type Bundle struct {
	ID          string `json:"id"`
	AgentID     string `json:"agent_id"`
	DisplayName string `json:"display_name"`
	Label       string `json:"label"`
	Files       []File `json:"files"`
	Available   bool   `json:"available"`
}

// Catalog is the host-aware catalog plus an internal lookup for transfer.
type Catalog struct {
	Bundles     []Bundle
	bundlesByID map[string]Bundle
}

// BuildCatalog creates a catalog for the current host home and OS.
func BuildCatalog(enabledAgents []agents.Agent) Catalog {
	homeDir, _ := os.UserHomeDir()
	return BuildCatalogForHost(enabledAgents, runtime.GOOS, homeDir)
}

// BuildCatalogForHost creates a catalog for an explicit host OS and home.
// Only agents with a PortableConfig declaration contribute bundles.
func BuildCatalogForHost(enabledAgents []agents.Agent, currentOS, homeDir string) Catalog {
	bundles := make([]Bundle, 0)
	byID := make(map[string]Bundle)
	for _, ag := range enabledAgents {
		capability, ok := ag.(agents.PortableConfigAgent)
		if !ok || capability.PortableConfig() == nil {
			continue
		}
		config := capability.PortableConfig()
		for _, declared := range config.Bundles {
			if declared.ID == "" || len(declared.Files) == 0 {
				continue
			}
			if _, duplicate := byID[declared.ID]; duplicate {
				continue
			}
			bundle := Bundle{
				ID:          declared.ID,
				AgentID:     ag.ID(),
				DisplayName: ag.DisplayName(),
				Label:       declared.Label,
				Files:       make([]File, 0, len(declared.Files)),
			}
			for _, declaredFile := range declared.Files {
				sourcePath := sourcePathForOS(declaredFile.SourcePaths, currentOS)
				if sourcePath == "" || declaredFile.TargetPath == "" {
					continue
				}
				available := localRegularFileExists(homeDir, sourcePath)
				bundle.Files = append(bundle.Files, File{
					SourcePath: sourcePath,
					TargetPath: declaredFile.TargetPath,
					Available:  available,
				})
				bundle.Available = bundle.Available || available
			}
			if len(bundle.Files) == 0 {
				continue
			}
			bundles = append(bundles, bundle)
			byID[bundle.ID] = bundle
		}
	}
	sort.Slice(bundles, func(i, j int) bool { return bundles[i].ID < bundles[j].ID })
	return Catalog{Bundles: bundles, bundlesByID: byID}
}

// FindBundle returns a declared bundle by its stable ID.
func (c Catalog) FindBundle(id string) (Bundle, bool) {
	bundle, ok := c.bundlesByID[id]
	return bundle, ok
}

func sourcePathForOS(paths map[string]string, currentOS string) string {
	if path := paths[currentOS]; path != "" {
		return path
	}
	if path := paths["linux"]; path != "" {
		return path
	}
	return paths[""]
}

func localRegularFileExists(homeDir, relativePath string) bool {
	if homeDir == "" {
		return false
	}
	info, err := os.Lstat(filepath.Join(homeDir, relativePath))
	return err == nil && info.Mode().IsRegular()
}
