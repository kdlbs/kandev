package compatibility

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const ModulePath = "github.com/kdlbs/kandev/pluginsdk"
const PackageName = "@kandev/plugin-sdk"

func ResolveGo(directory string) (Record, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-mod=readonly", "-m", "-json", ModulePath)
	cmd.Dir = directory
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != "GOWORK" && key != "GOFLAGS" {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GOWORK=off", "GOFLAGS=")
	data, err := cmd.Output()
	if err != nil {
		return Record{}, fmt.Errorf("resolve selected Go SDK (install dependencies first): %w", err)
	}
	var module struct {
		Path, Version, Dir string
		Replace            *json.RawMessage
	}
	if err = json.Unmarshal(data, &module); err != nil {
		return Record{}, fmt.Errorf("selected Go module evidence: %w", err)
	}
	if module.Replace != nil {
		return Record{}, fmt.Errorf("selected Go SDK replacement is not published dependency evidence")
	}
	if module.Path != ModulePath || module.Dir == "" || !strings.HasPrefix(module.Version, "v") {
		return Record{}, fmt.Errorf("missing selected Go SDK version or downloaded directory")
	}
	return ReadRecord(filepath.Join(module.Dir, "sdk-release.json"), strings.TrimPrefix(module.Version, "v"))
}

type packageManifest struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

func readJSON(path string, target any) error {
	data, err := readBounded(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
func (p packageManifest) sdkSpecifier() (string, error) {
	spec := ""
	for _, deps := range []map[string]string{p.Dependencies, p.DevDependencies, p.OptionalDependencies} {
		if value := deps[PackageName]; value != "" {
			if spec != "" && spec != value {
				return "", fmt.Errorf("ambiguous SDK dependency specifiers")
			}
			spec = value
		}
	}
	if spec == "" {
		return "", fmt.Errorf("package.json has no direct SDK dependency")
	}
	return spec, nil
}

func ResolveFrontend(directory, packagePath string) (Record, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return Record{}, err
	}
	if !filepath.IsAbs(packagePath) {
		packagePath = filepath.Join(directory, packagePath)
	}
	if filepath.Clean(packagePath) != filepath.Join(directory, "node_modules", "@kandev", "plugin-sdk") {
		return Record{}, fmt.Errorf("frontend SDK must be the installed node_modules/@kandev/plugin-sdk")
	}
	var installed, consumer packageManifest
	if err = readJSON(filepath.Join(packagePath, "package.json"), &installed); err != nil {
		return Record{}, fmt.Errorf("installed frontend SDK: %w", err)
	}
	if installed.Name != PackageName {
		return Record{}, fmt.Errorf("installed package is not %s", PackageName)
	}
	if err = readJSON(filepath.Join(directory, "package.json"), &consumer); err != nil {
		return Record{}, err
	}
	spec, err := consumer.sdkSpecifier()
	if err != nil {
		return Record{}, err
	}
	if err = registrySpecifier(spec, installed.Version); err != nil {
		return Record{}, err
	}
	lockRoot, kind, err := findLock(directory)
	if err != nil {
		return Record{}, err
	}
	if err = verifyInstalledPath(packagePath, lockRoot, installed.Version, kind); err != nil {
		return Record{}, err
	}
	if kind == "npm" {
		err = checkNPMLock(lockRoot, directory, spec, installed.Version)
	} else {
		err = checkPNPMLock(lockRoot, directory, spec, installed.Version)
	}
	if err != nil {
		return Record{}, err
	}
	return ReadRecord(filepath.Join(packagePath, "sdk-release.json"), installed.Version)
}

func findLock(directory string) (string, string, error) {
	for current := directory; ; current = filepath.Dir(current) {
		found := ""
		for _, entry := range []struct{ name, kind string }{{"package-lock.json", "npm"}, {"pnpm-lock.yaml", "pnpm"}} {
			_, err := os.Stat(filepath.Join(current, entry.name))
			if err == nil {
				if found != "" {
					return "", "", fmt.Errorf("ambiguous npm and pnpm lock evidence")
				}
				found = entry.kind
			} else if !os.IsNotExist(err) {
				return "", "", err
			}
		}
		if found != "" {
			return current, found, nil
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	return "", "", fmt.Errorf("missing supported package-lock.json or pnpm-lock.yaml evidence")
}
func verifyInstalledPath(path, root, version, kind string) error {
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if actual == filepath.Clean(path) {
		return nil
	}
	store := filepath.Join(root, "node_modules", ".pnpm", "@kandev+plugin-sdk@"+version, "node_modules", "@kandev", "plugin-sdk")
	if kind == "pnpm" && actual == store {
		return nil
	}
	return fmt.Errorf("frontend SDK is a local substitution outside the installed package or pnpm store")
}
