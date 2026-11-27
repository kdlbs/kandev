package mcpconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const nativeMCPWindowsOS = "windows"

func resolveExecutable(executable string, env map[string]string) (string, error) {
	if executable == "" {
		return "", ErrNativeMCPExecutableUnavailable
	}
	windows := runtime.GOOS == nativeMCPWindowsOS
	if hasExecutablePathSeparator(executable, windows) {
		for _, candidate := range executableCandidates(executable, env, windows) {
			if path, ok := usableExecutable(candidate, windows); ok {
				return path, nil
			}
		}
		return "", ErrNativeMCPExecutableUnavailable
	}

	searchPath, ok := environmentValue(env, "PATH", windows)
	if !ok {
		searchPath, _ = os.LookupEnv("PATH")
	}
	for _, directory := range filepath.SplitList(searchPath) {
		if directory == "" {
			continue
		}
		for _, candidate := range executableCandidates(filepath.Join(directory, executable), env, windows) {
			if path, ok := usableExecutable(candidate, windows); ok {
				return path, nil
			}
		}
	}
	return "", ErrNativeMCPExecutableUnavailable
}

func executableCandidates(path string, env map[string]string, windows bool) []string {
	if !windows || filepath.Ext(path) != "" {
		return []string{path}
	}
	extensions, ok := environmentValue(env, "PATHEXT", true)
	if !ok || extensions == "" {
		extensions = ".COM;.EXE;.BAT;.CMD"
	}
	values := strings.Split(extensions, ";")
	candidates := make([]string, 0, len(values))
	for _, extension := range values {
		if extension == "" {
			continue
		}
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		candidates = append(candidates, path+extension)
	}
	return candidates
}

func usableExecutable(candidate string, windows bool) (string, bool) {
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	if !windows && info.Mode().Perm()&0111 == 0 {
		return "", false
	}
	return absolute, true
}

func hasExecutablePathSeparator(value string, windows bool) bool {
	if strings.ContainsRune(value, filepath.Separator) {
		return true
	}
	return windows && strings.ContainsAny(value, `/\\`)
}

func environmentValue(env map[string]string, key string, caseInsensitive bool) (string, bool) {
	if value, ok := env[key]; ok {
		return value, true
	}
	if caseInsensitive {
		for candidate, value := range env {
			if strings.EqualFold(candidate, key) {
				return value, true
			}
		}
	}
	return "", false
}
