package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProviderErrorsCatalogFileFromYAMLAndEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("providerErrors:\n  catalogFile: rules.yaml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KANDEV_PROVIDER_ERRORS_CATALOG_FILE", "")
	if err := os.Unsetenv("KANDEV_PROVIDER_ERRORS_CATALOG_FILE"); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.ProviderErrors.CatalogFile != "rules.yaml" || cfg.SourceFor("providerErrors.catalogFile") != SourceConfiguration {
		t.Fatalf("yaml value = %q source %q", cfg.ProviderErrors.CatalogFile, cfg.SourceFor("providerErrors.catalogFile"))
	}

	t.Setenv("KANDEV_PROVIDER_ERRORS_CATALOG_FILE", "/etc/kandev/rules.json")
	cfg, err = LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.ProviderErrors.CatalogFile != "/etc/kandev/rules.json" || cfg.SourceFor("providerErrors.catalogFile") != SourceEnvironment {
		t.Fatalf("env value = %q source %q", cfg.ProviderErrors.CatalogFile, cfg.SourceFor("providerErrors.catalogFile"))
	}
}
