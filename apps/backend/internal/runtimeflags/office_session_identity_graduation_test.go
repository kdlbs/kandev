package runtimeflags

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
)

func TestOfficeSessionIdentityIsRetiredAndStaleValuesAreInert(t *testing.T) {
	const key = "features.officeSessionIdentity"
	const envVar = "KANDEV_FEATURES_OFFICE_SESSION_IDENTITY"
	if _, ok := DefinitionByKey(key); ok {
		t.Fatalf("retired runtime flag %q is still active", key)
	}
	if _, ok := ValuesFromConfig(&config.Config{})[key]; ok {
		t.Fatalf("retired runtime flag %q is still returned from config", key)
	}
	retired := false
	for _, identity := range retiredRuntimeFlagIdentities {
		if identity.key == key && identity.envVar == envVar {
			retired = true
			break
		}
	}
	if !retired {
		t.Fatalf("retired identity pair %q / %q is missing", key, envVar)
	}

	t.Setenv(envVar, "false")
	cfg := &config.Config{}
	ApplyStatesToConfig(cfg, []RuntimeFlagState{{Key: key, EffectiveValue: false}})
	if _, active := OptionsFromConfig(cfg).EnvValues[envVar]; active {
		t.Fatalf("retired environment variable %q is still read by runtime flags", envVar)
	}
	encoded, err := json.Marshal(cfg.Features)
	if err != nil {
		t.Fatalf("marshal feature response: %v", err)
	}
	if strings.Contains(string(encoded), "officeSessionIdentity") {
		t.Fatalf("retired feature remains in /api/v1/features shape: %s", encoded)
	}

	repoRoot := officeSessionIdentityRepoRoot(t)
	for _, relPath := range []string{
		"profiles.yaml",
		"apps/backend/internal/profiles/profiles.yaml",
		"apps/backend/internal/common/config/catalog.go",
	} {
		content, err := os.ReadFile(filepath.Join(repoRoot, relPath))
		if err != nil {
			t.Fatalf("read %s: %v", relPath, err)
		}
		if strings.Contains(string(content), envVar) {
			t.Fatalf("retired environment variable %q remains in %s", envVar, relPath)
		}
	}
}

func officeSessionIdentityRepoRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// sourceFile: apps/backend/internal/runtimeflags/<this file> -> repo root
	// is four directories up.
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../../.."))
}
