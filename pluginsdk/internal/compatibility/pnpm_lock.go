package compatibility

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type pnpmDependency struct {
	Specifier string `yaml:"specifier"`
	Version   string `yaml:"version"`
}
type pnpmImporter struct {
	Dependencies         map[string]pnpmDependency `yaml:"dependencies"`
	DevDependencies      map[string]pnpmDependency `yaml:"devDependencies"`
	OptionalDependencies map[string]pnpmDependency `yaml:"optionalDependencies"`
}

func (p pnpmImporter) sdk() (pnpmDependency, error) {
	var found pnpmDependency
	for _, deps := range []map[string]pnpmDependency{p.Dependencies, p.DevDependencies, p.OptionalDependencies} {
		if entry, ok := deps[PackageName]; ok {
			if found.Specifier != "" && entry != found {
				return found, fmt.Errorf("ambiguous pnpm SDK dependency")
			}
			found = entry
		}
	}
	return found, nil
}
func checkPNPMLock(root, directory, spec, version string) error {
	var lock struct {
		Version   string                  `yaml:"lockfileVersion"`
		Importers map[string]pnpmImporter `yaml:"importers"`
		Packages  map[string]struct {
			Resolution struct {
				Integrity string `yaml:"integrity"`
				Tarball   string `yaml:"tarball"`
			} `yaml:"resolution"`
		} `yaml:"packages"`
		Snapshots map[string]any `yaml:"snapshots"`
	}
	data, err := readBounded(filepath.Join(root, "pnpm-lock.yaml"), lockfileLimit)
	if err != nil {
		return err
	}
	if err = yaml.Unmarshal(data, &lock); err != nil {
		return fmt.Errorf("pnpm lock: %w", err)
	}
	if lock.Version != "9.0" {
		return fmt.Errorf("unsupported pnpm lockfileVersion %s (expected 9.0)", lock.Version)
	}
	importer, err := lockImporter(root, directory)
	if err != nil {
		return err
	}
	entry, err := lock.Importers[importer].sdk()
	if err != nil {
		return err
	}
	if strings.HasPrefix(entry.Version, "link:") || strings.HasPrefix(entry.Version, "file:") {
		return fmt.Errorf("pnpm SDK must use a registry resolution")
	}
	if entry.Specifier != spec || entry.Version != version {
		return fmt.Errorf("pnpm lock SDK version or specifier differs from installed package")
	}
	key := PackageName + "@" + version
	pkg, ok := lock.Packages[key]
	if !ok {
		return fmt.Errorf("pnpm lock has no SDK package resolution")
	}
	if _, ok = lock.Snapshots[key]; !ok {
		return fmt.Errorf("pnpm lock has no SDK snapshot")
	}
	tarball := pkg.Resolution.Tarball
	if tarball == "" {
		tarball = "https://registry.npmjs.org/@kdlbs/plugin-sdk/-/plugin-sdk-" + version + ".tgz"
	}
	return registryResolution(tarball, pkg.Resolution.Integrity)
}
