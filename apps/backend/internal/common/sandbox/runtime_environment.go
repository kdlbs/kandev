package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type RuntimeDirectories struct {
	Home   string `json:"home"`
	Config string `json:"config"`
	State  string `json:"state"`
	Temp   string `json:"temp"`
}

func IsolatedEnvironment(final map[string]string, allowed []string, directories RuntimeDirectories) ([]string, error) {
	keys, err := canonicalEnvironmentKeys(allowed)
	if err != nil {
		return nil, err
	}
	keys = append(keys, environmentPath, "LANG", "LC_ALL", "TERM", "COLORTERM", "TZ")
	values := make(map[string]string, len(keys)+4)
	for _, key := range keys {
		if value, exists := final[key]; exists {
			values[key] = value
		}
	}
	for key, dir := range map[string]string{environmentHome: directories.Home, environmentConfigHome: directories.Config, environmentStateHome: directories.State, environmentTempDir: directories.Temp} {
		if !validRuntimeDirectory(dir) {
			return nil, ErrInvalidPolicy
		}
		values[key] = dir
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		if strings.ContainsRune(value, 0) {
			return nil, ErrInvalidPolicy
		}
		result = append(result, key+"="+value)
	}
	slices.Sort(result)
	return result, nil
}

func validRuntimeDirectory(dir string) bool {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return false
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil || canonical != dir {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir() && info.Mode().Perm()&0077 == 0
}
