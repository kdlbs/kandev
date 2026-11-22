package compatibility

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
)

func registrySpecifier(spec, version string) error {
	constraint, err := semver.NewConstraint(spec)
	if err != nil {
		return fmt.Errorf("SDK dependency must use a registry version constraint, got %q", spec)
	}
	selected, err := stableVersion("installed SDK version", version)
	if err != nil {
		return err
	}
	if !constraint.Check(selected) {
		return fmt.Errorf("installed SDK version %s does not satisfy %s", version, spec)
	}
	return nil
}
func registryResolution(resolved, integrity string) error {
	location, err := url.Parse(resolved)
	if err != nil || (location.Scheme != "https" && location.Scheme != "http") || location.Host == "" || location.User != nil {
		return fmt.Errorf("SDK lock resolution must identify a registry archive, got %q", resolved)
	}
	digest, decodeErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(integrity, "sha512-"))
	if !strings.HasPrefix(integrity, "sha512-") || decodeErr != nil || len(digest) != 64 {
		return fmt.Errorf("SDK registry lock evidence has no SHA-512 integrity")
	}
	return nil
}
func lockImporter(root, directory string) (string, error) {
	relative, err := filepath.Rel(root, directory)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(relative), nil
}

type npmLockedPackage struct {
	Version              string            `json:"version"`
	Resolved             string            `json:"resolved"`
	Integrity            string            `json:"integrity"`
	Link                 bool              `json:"link"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

func checkNPMLock(root, directory, packagePath, spec, version string) error {
	var lock struct {
		Version  int                         `json:"lockfileVersion"`
		Packages map[string]npmLockedPackage `json:"packages"`
	}
	if err := readJSON(filepath.Join(root, "package-lock.json"), &lock, lockfileLimit); err != nil {
		return fmt.Errorf("npm lock: %w", err)
	}
	if lock.Version != 2 && lock.Version != 3 {
		return fmt.Errorf("unsupported npm lockfileVersion %d (expected 2 or 3)", lock.Version)
	}
	importer, err := lockImporter(root, directory)
	if err != nil {
		return err
	}
	if importer == "." {
		importer = ""
	}
	entry := lock.Packages[importer]
	declared, err := (packageManifest{Dependencies: entry.Dependencies, DevDependencies: entry.DevDependencies, OptionalDependencies: entry.OptionalDependencies}).sdkSpecifier()
	if err != nil || declared != spec {
		return fmt.Errorf("npm lock importer differs from package.json SDK dependency")
	}
	key, err := lockImporter(root, packagePath)
	if err != nil {
		return err
	}
	installed, ok := lock.Packages[key]
	if !ok || installed.Link {
		return fmt.Errorf("npm lock has no registry SDK resolution")
	}
	if installed.Version != version {
		return fmt.Errorf("npm lock version %s differs from installed version %s", installed.Version, version)
	}
	return registryResolution(installed.Resolved, installed.Integrity)
}
