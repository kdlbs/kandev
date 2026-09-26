package installer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const windowsOS = "windows"

func resolveCommand(binary string, runner CommandRunner) (string, map[string]string, error) {
	env, overrides, err := commandEnvironment(runner)
	if err != nil {
		return "", nil, err
	}
	path, err := lookPathInEnvironment(binary, env)
	return path, overrides, err
}

// FindCommand resolves a binary with the same task environment used by the
// supplied command runner.
func FindCommand(binary string, runner CommandRunner) (string, error) {
	env, _, err := commandEnvironment(runner)
	if err != nil {
		return "", err
	}
	return lookPathInEnvironment(binary, env)
}

// FindCommandInDirectory resolves a binary in one trusted directory with the
// task environment's platform-specific executable extensions.
func FindCommandInDirectory(binary, directory string, runner CommandRunner) (string, error) {
	env, _, err := commandEnvironment(runner)
	if err != nil {
		return "", err
	}
	return findCommandInDirectory(binary, directory, env)
}

// CommandEnvironmentValue reads one value from the same merged process and
// task environment used for command lookup and execution.
func CommandEnvironmentValue(runner CommandRunner, key string) (string, error) {
	env, _, err := commandEnvironment(runner)
	if err != nil {
		return "", err
	}
	return environmentValue(env, key), nil
}

func commandEnvironment(runner CommandRunner) (map[string]string, map[string]string, error) {
	env := processEnvironment()
	if runner == nil {
		return env, nil, nil
	}
	overrides, err := runner.CommandEnvironment()
	if err != nil {
		return nil, nil, err
	}
	for key, value := range overrides {
		setEnvironmentValue(env, key, value)
	}
	return env, overrides, nil
}

func processEnvironment() map[string]string {
	env := make(map[string]string, len(os.Environ()))
	for _, entry := range os.Environ() {
		if separator := strings.IndexByte(entry, '='); separator > 0 {
			setEnvironmentValue(env, entry[:separator], entry[separator+1:])
		}
	}
	return env
}

func setEnvironmentValue(env map[string]string, key, value string) {
	if runtime.GOOS == windowsOS {
		for existing := range env {
			if strings.EqualFold(existing, key) {
				delete(env, existing)
			}
		}
	}
	env[key] = value
}

func environmentValue(env map[string]string, key string) string {
	if value, ok := env[key]; ok {
		return value
	}
	if runtime.GOOS == windowsOS {
		for existing, value := range env {
			if strings.EqualFold(existing, key) {
				return value
			}
		}
	}
	return ""
}

func lookPathInEnvironment(binary string, env map[string]string) (string, error) {
	if filepath.IsAbs(binary) || strings.ContainsRune(binary, filepath.Separator) {
		if isExecutableFile(binary) {
			return filepath.Abs(binary)
		}
		return "", &exec.Error{Name: binary, Err: exec.ErrNotFound}
	}
	for _, directory := range filepath.SplitList(environmentValue(env, "PATH")) {
		if directory == "" || !filepath.IsAbs(directory) {
			continue
		}
		if candidate, err := findCommandInDirectory(binary, directory, env); err == nil {
			return candidate, nil
		}
	}
	return "", &exec.Error{Name: binary, Err: exec.ErrNotFound}
}

func findCommandInDirectory(binary, directory string, env map[string]string) (string, error) {
	if !filepath.IsAbs(directory) {
		return "", &exec.Error{Name: binary, Err: exec.ErrNotFound}
	}
	for _, name := range executableNames(binary, env) {
		candidate := filepath.Join(directory, name)
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}
	return "", &exec.Error{Name: binary, Err: exec.ErrNotFound}
}

func executableNames(binary string, env map[string]string) []string {
	if runtime.GOOS != windowsOS || filepath.Ext(binary) != "" {
		return []string{binary}
	}
	extensions := environmentValue(env, "PATHEXT")
	if extensions == "" {
		extensions = ".COM;.EXE;.BAT;.CMD"
	}
	names := make([]string, 0, 4)
	for _, extension := range strings.Split(extensions, ";") {
		if extension != "" {
			names = append(names, binary+strings.ToLower(extension))
		}
	}
	return names
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return runtime.GOOS == windowsOS || info.Mode().Perm()&0o111 != 0
}

func combinedOutput(ctx context.Context, runner CommandRunner, spec CommandSpec) ([]byte, error) {
	if runner != nil {
		return runner.CombinedOutput(ctx, spec)
	}
	cmd := exec.CommandContext(ctx, spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	if len(spec.Env) > 0 {
		cmd.Env = os.Environ()
		for key, value := range spec.Env {
			cmd.Env = upsertCommandEnv(cmd.Env, key, value)
		}
	}
	return cmd.CombinedOutput()
}

func upsertCommandEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
