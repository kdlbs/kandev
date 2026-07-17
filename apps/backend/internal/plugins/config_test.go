package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	goruntime "runtime"
	"testing"

	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/internal/plugins/store"
)

// testConfigSchema mirrors the JSON shape a manifest's config_schema decodes
// to: a JSON-Schema-like object with a secret token field (the GitHub-PAT
// case), a plain string, an integer, an enum, and a boolean.
func testConfigSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []any{"github_token"},
		"properties": map[string]any{
			"github_token": map[string]any{"type": "string", "secret": true},
			"webhook_key":  map[string]any{"type": "string", "format": "password"},
			"org":          map[string]any{"type": "string"},
			"max_items":    map[string]any{"type": "integer"},
			"channel":      map[string]any{"type": "string", "enum": []any{"dev", "ops"}},
			"verbose":      map[string]any{"type": "boolean"},
		},
	}
}

// testPackageWithConfigSchema builds a valid runtime-managed package whose
// manifest declares the config schema above.
func testPackageWithConfigSchema(t *testing.T, id string) *bytes.Buffer {
	t.Helper()
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: 1.0.0
display_name: Test Plugin
capabilities:
  state: true
config_schema:
  type: object
  required: ["github_token"]
  properties:
    github_token:
      type: string
      secret: true
    webhook_key:
      type: string
      format: password
    org:
      type: string
    max_items:
      type: integer
    channel:
      type: string
      enum: ["dev", "ops"]
    verbose:
      type: boolean
runtime:
  type: binary
  executables:
    %s: server/plugin
`, id, platformKey)

	var buf bytes.Buffer
	files := map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}
	if err := pkgtartest.WritePackage(&buf, files); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &buf
}

func installConfigPlugin(t *testing.T, svc *Service, id string) *store.Record {
	t.Helper()
	rec, err := svc.Install(context.Background(), testPackageWithConfigSchema(t, id))
	if err != nil {
		t.Fatalf("Install(%q): %v", id, err)
	}
	return rec
}

// --- helper unit tests ---

func TestSecretPropertyKeysDetectsSecretAndPasswordFormat(t *testing.T) {
	keys := secretPropertyKeys(testConfigSchema())
	if !keys["github_token"] || !keys["webhook_key"] {
		t.Fatalf("secretPropertyKeys() = %v, want github_token and webhook_key", keys)
	}
	if keys["org"] {
		t.Fatalf("secretPropertyKeys() flagged non-secret field org")
	}
}

func TestMaskSecretsMasksOnlyNonEmptySecretStrings(t *testing.T) {
	masked := maskSecrets(map[string]any{
		"github_token": "ghp_real",
		"webhook_key":  "",
		"org":          "kdlbs",
	}, testConfigSchema())

	if masked["github_token"] != configSecretMask {
		t.Fatalf("github_token = %v, want mask", masked["github_token"])
	}
	if masked["webhook_key"] != "" {
		t.Fatalf("empty secret should stay empty, got %v", masked["webhook_key"])
	}
	if masked["org"] != "kdlbs" {
		t.Fatalf("org = %v, want kdlbs", masked["org"])
	}
}

func TestMergeMaskedSecretsKeepsStoredValueForMask(t *testing.T) {
	merged := mergeMaskedSecrets(
		map[string]any{"github_token": configSecretMask, "org": "new-org"},
		map[string]any{"github_token": "ghp_real", "org": "old-org"},
		testConfigSchema(),
	)
	if merged["github_token"] != "ghp_real" {
		t.Fatalf("github_token = %v, want stored ghp_real", merged["github_token"])
	}
	if merged["org"] != "new-org" {
		t.Fatalf("org = %v, want new-org (full replace for non-secrets)", merged["org"])
	}
}

func TestMergeMaskedSecretsDropsMaskWithNoStoredValue(t *testing.T) {
	merged := mergeMaskedSecrets(
		map[string]any{"github_token": configSecretMask},
		map[string]any{},
		testConfigSchema(),
	)
	if _, present := merged["github_token"]; present {
		t.Fatalf("mask with no stored value should be dropped, got %v", merged)
	}
}

func TestValidateConfigSchema(t *testing.T) {
	schema := testConfigSchema()
	cases := []struct {
		name    string
		config  map[string]any
		wantErr bool
	}{
		{"valid full", map[string]any{
			"github_token": "ghp_x", "org": "kdlbs", "max_items": float64(10),
			"channel": "dev", "verbose": true,
		}, false},
		{"missing required", map[string]any{"org": "kdlbs"}, true},
		{"wrong string type", map[string]any{"github_token": "x", "org": float64(3)}, true},
		{"wrong boolean type", map[string]any{"github_token": "x", "verbose": "yes"}, true},
		{"non-integral integer", map[string]any{"github_token": "x", "max_items": 2.5}, true},
		{"integral float accepted", map[string]any{"github_token": "x", "max_items": float64(5)}, false},
		{"yaml int accepted", map[string]any{"github_token": "x", "max_items": 5}, false},
		{"enum mismatch", map[string]any{"github_token": "x", "channel": "prod"}, true},
		{"undeclared key allowed", map[string]any{"github_token": "x", "extra": "ok"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfigSchema("test-plugin", tc.config, schema)
			if tc.wantErr && !errors.Is(err, ErrConfigInvalid) {
				t.Fatalf("error = %v, want ErrConfigInvalid", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateConfigSchemaNilSchemaIsPermissive(t *testing.T) {
	if err := validateConfigSchema("test-plugin", map[string]any{"anything": 1}, nil); err != nil {
		t.Fatalf("nil schema should accept anything, got %v", err)
	}
}

// --- service tests ---

func TestServiceGetMaskedConfigMasksSecrets(t *testing.T) {
	svc, fsStore, vault := newTestServiceWithVault(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")

	err := svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{
		"github_token": "ghp_real", "org": "kdlbs",
	})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	masked, err := svc.GetMaskedConfig("kandev-plugin-github")
	if err != nil {
		t.Fatalf("GetMaskedConfig: %v", err)
	}
	if masked["github_token"] != configSecretMask {
		t.Fatalf("masked github_token = %v, want mask", masked["github_token"])
	}
	if masked["org"] != "kdlbs" {
		t.Fatalf("masked org = %v, want kdlbs", masked["org"])
	}

	// The config file holds a vault ref, never the cleartext; the vault holds
	// the cleartext.
	stored, err := fsStore.GetConfig("kandev-plugin-github")
	if err != nil {
		t.Fatalf("store GetConfig: %v", err)
	}
	if stored["github_token"] != configVaultRef("kandev-plugin-github", "github_token") {
		t.Fatalf("stored github_token = %v, want vault ref", stored["github_token"])
	}
	if v, _ := vault.get(pluginConfigSecretID("kandev-plugin-github", "github_token")); v != "ghp_real" {
		t.Fatalf("vault value = %q, want cleartext ghp_real", v)
	}
}

func TestServiceUpdateConfigPreservesMaskedSecret(t *testing.T) {
	svc, fsStore, vault := newTestServiceWithVault(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("UpdateConfig: %v", err)
		}
	}
	must(svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{"github_token": "ghp_real"}))
	// Re-submitting the form: token comes back as the mask, org changes.
	must(svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{
		"github_token": configSecretMask, "org": "kdlbs",
	}))

	// The masked round trip keeps the vault's cleartext value; org updates.
	if v, _ := vault.get(pluginConfigSecretID("kandev-plugin-github", "github_token")); v != "ghp_real" {
		t.Fatalf("vault value = %q, want preserved ghp_real", v)
	}
	stored, err := fsStore.GetConfig("kandev-plugin-github")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if stored["org"] != "kdlbs" {
		t.Fatalf("stored org = %v, want kdlbs", stored["org"])
	}
}

func TestServiceUpdateConfigInvalidRejectedAndNotPersisted(t *testing.T) {
	svc, fsStore, _ := newTestService(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")

	err := svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{"org": "no-token"})
	if !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("error = %v, want ErrConfigInvalid", err)
	}
	stored, err := fsStore.GetConfig("kandev-plugin-github")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("invalid config must not persist, got %v", stored)
	}
}

func TestServiceUpdateConfigRestartsRunningPlugin(t *testing.T) {
	svc, _, rt := newTestService(t)
	svc.SetSecrets(newFakeSecretRevealer())
	installConfigPlugin(t, svc, "kandev-plugin-github") // Install activates -> running

	if err := svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{"github_token": "ghp_x"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if !rt.stopped("kandev-plugin-github") {
		t.Fatalf("running plugin should be stopped on config change")
	}
	if got := rt.startCallCount("kandev-plugin-github"); got != 2 {
		t.Fatalf("start calls = %d, want 2 (install + config restart)", got)
	}
	rec, _ := svc.Get("kandev-plugin-github")
	if rec.Status != StatusActive {
		t.Fatalf("status = %q, want active after restart", rec.Status)
	}
}

func TestServiceUpdateConfigDoesNotSpawnStoppedPlugin(t *testing.T) {
	svc, _, rt := newTestService(t)
	svc.SetSecrets(newFakeSecretRevealer())
	installConfigPlugin(t, svc, "kandev-plugin-github")
	if err := svc.Disable("kandev-plugin-github"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	before := rt.startCallCount("kandev-plugin-github")

	if err := svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{"github_token": "ghp_x"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if got := rt.startCallCount("kandev-plugin-github"); got != before {
		t.Fatalf("start calls = %d, want %d (no spawn for a stopped plugin)", got, before)
	}
}

func TestServiceUpdateConfigRestartFailurePersistsConfigAndSetsError(t *testing.T) {
	svc, fsStore, rt := newTestService(t)
	vault := newFakeSecretRevealer()
	svc.SetSecrets(vault)
	installConfigPlugin(t, svc, "kandev-plugin-github")
	rt.setStartErr("kandev-plugin-github", errors.New("spawn boom"))

	err := svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{"github_token": "ghp_x"})
	if err == nil {
		t.Fatalf("UpdateConfig should surface the restart failure")
	}
	// The config commit and vault write succeeded (only the restart failed):
	// the config file keeps the ref, the vault keeps the value.
	stored, err := fsStore.GetConfig("kandev-plugin-github")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if stored["github_token"] != configVaultRef("kandev-plugin-github", "github_token") {
		t.Fatalf("config should persist despite restart failure, got %v", stored)
	}
	if v, _ := vault.get(pluginConfigSecretID("kandev-plugin-github", "github_token")); v != "ghp_x" {
		t.Fatalf("vault value = %q, want ghp_x", v)
	}
	rec, _ := svc.Get("kandev-plugin-github")
	if rec.Status != StatusError {
		t.Fatalf("status = %q, want error after failed restart", rec.Status)
	}
}

// --- host RPC tests ---

func TestPluginHostGetConfigReturnsCleartextConfig(t *testing.T) {
	svc, fsStore, vault := newTestServiceWithVault(t)
	rec := installConfigPlugin(t, svc, "kandev-plugin-github")
	if err := svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{"github_token": "ghp_real"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	// The host resolves the config file's vault ref back to cleartext for the
	// plugin process.
	host := &pluginHost{
		pluginID:     "kandev-plugin-github",
		configSchema: rec.ConfigSchema,
		configs:      fsStore,
		secrets:      vault,
	}
	config, err := host.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config["github_token"] != "ghp_real" {
		t.Fatalf("host GetConfig github_token = %v, want cleartext ghp_real", config["github_token"])
	}
}

func TestPluginHostGetConfigWithoutStoreReturnsEmpty(t *testing.T) {
	host := &pluginHost{pluginID: "p"}
	config, err := host.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config == nil || len(config) != 0 {
		t.Fatalf("GetConfig = %v, want empty non-nil map", config)
	}
}

// --- handler tests ---

func TestGetConfigHandlerReturnsMaskedConfig(t *testing.T) {
	router, svc := newTestRouter(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")
	if err := svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{"github_token": "ghp_real"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	rec := doRequest(router, http.MethodGet, "/api/plugins/kandev-plugin-github/config", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !bytes.Contains([]byte(body), []byte(configSecretMask)) {
		t.Fatalf("body should carry the mask, got %s", body)
	}
	if bytes.Contains([]byte(body), []byte("ghp_real")) {
		t.Fatalf("cleartext secret leaked on the operator API: %s", body)
	}
}

func TestGetConfigHandlerMissingReturns404(t *testing.T) {
	router, _ := newTestRouter(t)
	rec := doRequest(router, http.MethodGet, "/api/plugins/missing/config", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestUpdateConfigHandlerInvalidSchemaReturns400(t *testing.T) {
	router, svc := newTestRouter(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")

	rec := doRequest(router, http.MethodPatch, "/api/plugins/kandev-plugin-github",
		`{"config":{"org":"missing-token"}}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}
