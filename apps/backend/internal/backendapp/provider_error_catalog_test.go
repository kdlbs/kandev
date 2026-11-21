package backendapp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestProviderErrorCatalogPath(t *testing.T) {
	dir := t.TempDir()
	absolute := filepath.Join(dir, "rules.yaml")
	cases := []struct {
		name, file, source, want string
	}{
		{"unset", "", filepath.Join(dir, "config.yaml"), ""},
		{"absolute", absolute, filepath.Join(dir, "other", "config.yaml"), absolute},
		{"relative to config file", "rules.yaml", filepath.Join(dir, "config.yaml"), absolute},
		{"relative without config file", "rules.yaml", "", "rules.yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.ProviderErrors.CatalogFile = tc.file
			cfg.Source.FilePath = tc.source
			if got := providerErrorCatalogPath(cfg); got != tc.want {
				t.Fatalf("path = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInstallProviderErrorCatalog(t *testing.T) {
	t.Cleanup(func() { routingerr.SetExternalCatalog(nil) })
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(`{"version":1,"rules":[{"id":"t.credit",`+
		`"match":{"pattern":"credit balance exhausted"},"result":{"code":"quota_limited","confidence":"high"}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Source.FilePath = filepath.Join(dir, "config.yaml")
	cfg.ProviderErrors.CatalogFile = "rules.json"
	installProviderErrorCatalog(cfg, logger.Default())
	in := routingerr.Input{Phase: routingerr.PhaseStreaming, ProviderID: "gw", Stderr: "Error: credit balance exhausted"}
	if got := routingerr.Classify(in); got.ClassifierRule != "external_catalog.t.credit" {
		t.Fatalf("loaded catalog not consumed: %s/%s", got.Code, got.ClassifierRule)
	}

	cfg.ProviderErrors.CatalogFile = "missing.yaml"
	installProviderErrorCatalog(cfg, logger.Default())
	if got := routingerr.Classify(in); got.ClassifierRule == "external_catalog.t.credit" {
		t.Fatal("rejected catalog left the previous rules installed")
	}
}
