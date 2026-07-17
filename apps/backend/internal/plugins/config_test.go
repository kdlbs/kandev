package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	goruntime "runtime"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
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
			err := validateConfigSchema(tc.config, schema)
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
	if err := validateConfigSchema(map[string]any{"anything": 1}, nil); err != nil {
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

// --- vault-backed config + plugin-scoped secret tests ---

func newTestServiceWithVault(t *testing.T) (*Service, *store.FSStore, *fakeSecretRevealer) {
	t.Helper()
	svc, fsStore, _ := newTestService(t)
	vault := newFakeSecretRevealer()
	svc.SetSecrets(vault)
	return svc, fsStore, vault
}

func TestServiceUpdateConfigStoresSecretInVaultNotConfigFile(t *testing.T) {
	svc, fsStore, vault := newTestServiceWithVault(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")

	err := svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{
		"github_token": "ghp_real", "org": "kdlbs",
	})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	stored, err := fsStore.GetConfig("kandev-plugin-github")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	wantRef := configVaultRef("kandev-plugin-github", "github_token")
	if stored["github_token"] != wantRef {
		t.Fatalf("stored github_token = %v, want vault ref %q", stored["github_token"], wantRef)
	}
	if value, ok := vault.get(pluginConfigSecretID("kandev-plugin-github", "github_token")); !ok || value != "ghp_real" {
		t.Fatalf("vault entry = %q (found=%v), want cleartext ghp_real", value, ok)
	}

	masked, err := svc.GetMaskedConfig("kandev-plugin-github")
	if err != nil {
		t.Fatalf("GetMaskedConfig: %v", err)
	}
	if masked["github_token"] != configSecretMask {
		t.Fatalf("masked github_token = %v, want mask", masked["github_token"])
	}
}

func TestServiceUpdateConfigMaskRoundTripKeepsVaultValue(t *testing.T) {
	svc, fsStore, vault := newTestServiceWithVault(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")

	ctx := context.Background()
	if err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{"github_token": "ghp_real"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	// Re-submit with the mask: the ref stays a ref, the vault keeps the value.
	if err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{
		"github_token": configSecretMask, "org": "kdlbs",
	}); err != nil {
		t.Fatalf("UpdateConfig (mask round trip): %v", err)
	}

	stored, err := fsStore.GetConfig("kandev-plugin-github")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if stored["github_token"] != configVaultRef("kandev-plugin-github", "github_token") {
		t.Fatalf("stored github_token = %v, want vault ref", stored["github_token"])
	}
	if value, _ := vault.get(pluginConfigSecretID("kandev-plugin-github", "github_token")); value != "ghp_real" {
		t.Fatalf("vault value = %q, want preserved ghp_real", value)
	}
}

func TestServiceUpdateConfigRemovedSecretDeletesVaultEntry(t *testing.T) {
	svc, _, vault := newTestServiceWithVault(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")

	ctx := context.Background()
	if err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{"github_token": "ghp_real"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	// github_token is required by the schema, so drop the optional secret
	// path via a schema-less check: submit without the field entirely is
	// rejected; instead prove deletion through Uninstall below and via the
	// optional webhook_key secret here.
	if err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{
		"github_token": configSecretMask, "webhook_key": "whsec_1",
	}); err != nil {
		t.Fatalf("UpdateConfig (add webhook_key): %v", err)
	}
	if _, ok := vault.get(pluginConfigSecretID("kandev-plugin-github", "webhook_key")); !ok {
		t.Fatalf("webhook_key should be in the vault")
	}
	if err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{
		"github_token": configSecretMask,
	}); err != nil {
		t.Fatalf("UpdateConfig (remove webhook_key): %v", err)
	}
	if _, ok := vault.get(pluginConfigSecretID("kandev-plugin-github", "webhook_key")); ok {
		t.Fatalf("removed secret field should be deleted from the vault")
	}
}

func TestServiceUninstallPurgesPluginVaultNamespace(t *testing.T) {
	svc, _, vault := newTestServiceWithVault(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")

	ctx := context.Background()
	if err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{"github_token": "ghp_real"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	vault.set(pluginSecretID("kandev-plugin-github", "own-key"), "own-value")
	vault.set("unrelated", "keep-me")

	if err := svc.Uninstall(context.Background(), "kandev-plugin-github"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, ok := vault.get(pluginConfigSecretID("kandev-plugin-github", "github_token")); ok {
		t.Fatalf("config secret should be purged on uninstall")
	}
	if _, ok := vault.get(pluginSecretID("kandev-plugin-github", "own-key")); ok {
		t.Fatalf("plugin-owned secret should be purged on uninstall")
	}
	if _, ok := vault.get("unrelated"); !ok {
		t.Fatalf("secrets outside the plugin namespace must survive uninstall")
	}
}

func TestPluginHostGetConfigResolvesVaultRef(t *testing.T) {
	svc, fsStore, vault := newTestServiceWithVault(t)
	rec := installConfigPlugin(t, svc, "kandev-plugin-github")

	ctx := context.Background()
	if err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{"github_token": "ghp_real"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	host := &pluginHost{
		pluginID:     "kandev-plugin-github",
		configSchema: rec.ConfigSchema,
		configs:      fsStore,
		secrets:      vault,
	}
	config, err := host.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config["github_token"] != "ghp_real" {
		t.Fatalf("host GetConfig github_token = %v, want resolved cleartext", config["github_token"])
	}
}

func TestPluginHostSecretPrimitives(t *testing.T) {
	vault := newFakeSecretRevealer()
	host := &pluginHost{
		pluginID:     "kandev-plugin-github",
		capabilities: manifestCapsWithSecrets(),
		secrets:      vault,
	}
	ctx := context.Background()

	_, found, err := host.GetSecret(ctx, "pat")
	if err != nil || found {
		t.Fatalf("GetSecret(missing) = found=%v err=%v, want false,nil", found, err)
	}
	if err := host.SetSecret(ctx, "pat", "ghp_owned"); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	if value, ok := vault.get("plugin:kandev-plugin-github:secret:pat"); !ok || value != "ghp_owned" {
		t.Fatalf("vault entry = %q (found=%v), want namespaced ghp_owned", value, ok)
	}
	value, found, err := host.GetSecret(ctx, "pat")
	if err != nil || !found || value != "ghp_owned" {
		t.Fatalf("GetSecret = %q,%v,%v, want ghp_owned,true,nil", value, found, err)
	}
	if err := host.DeleteSecret(ctx, "pat"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	if err := host.DeleteSecret(ctx, "pat"); err != nil {
		t.Fatalf("DeleteSecret(missing) should be a no-op, got %v", err)
	}
}

func TestPluginHostSecretPrimitivesRequireCapabilityAndValidKey(t *testing.T) {
	host := &pluginHost{pluginID: "p", secrets: newFakeSecretRevealer()}
	if err := host.SetSecret(context.Background(), "k", "v"); err == nil {
		t.Fatalf("SetSecret without secrets capability should be denied")
	}

	host.capabilities = manifestCapsWithSecrets()
	for _, bad := range []string{"", "a b", "x:y", "../etc", ".hidden"} {
		if err := host.SetSecret(context.Background(), bad, "v"); err == nil {
			t.Fatalf("SetSecret(%q) should reject invalid key", bad)
		}
	}
}

func manifestCapsWithSecrets() manifest.Capabilities {
	return manifest.Capabilities{Secrets: true}
}

func TestMaskSecretsMasksNonStringSecretValues(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"pin":     map[string]any{"type": "integer", "secret": true},
			"enabled": map[string]any{"type": "boolean", "secret": true},
		},
	}
	masked := maskSecrets(map[string]any{"pin": 1234, "enabled": true}, schema)
	if masked["pin"] != configSecretMask || masked["enabled"] != configSecretMask {
		t.Fatalf("non-string secrets must be masked, got %v", masked)
	}
	// Zero values stay visible so the UI can tell "not set" apart.
	masked = maskSecrets(map[string]any{"pin": 0, "enabled": false}, schema)
	if masked["pin"] != 0 || masked["enabled"] != false {
		t.Fatalf("zero-value secrets should pass through, got %v", masked)
	}
}

func TestValidateConfigSchemaNumericEnumAcceptsJSONFloat(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			// Manifest YAML decodes these as int...
			"level": map[string]any{"type": "integer", "enum": []any{1, 2, 3}},
		},
	}
	// ...while an HTTP JSON submit arrives as float64.
	if err := validateConfigSchema(map[string]any{"level": float64(2)}, schema); err != nil {
		t.Fatalf("numeric enum should accept float64(2) against int enum: %v", err)
	}
	if err := validateConfigSchema(map[string]any{"level": float64(9)}, schema); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("out-of-enum numeric should still be rejected, got %v", err)
	}
}

func TestValidateConfigSchemaRejectsNonStringSecretValues(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"pin": map[string]any{"type": "integer", "secret": true},
		},
	}
	// A non-string secret would bypass vault storage and persist cleartext —
	// it must be rejected before it can ever reach the store.
	if err := validateConfigSchema(map[string]any{"pin": float64(1234)}, schema); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("non-string secret must be rejected, got %v", err)
	}
	// Absent stays allowed (an explicit null is already rejected by the
	// property type check — clients omit unset keys).
	if err := validateConfigSchema(map[string]any{}, schema); err != nil {
		t.Fatalf("absent secret should be fine, got %v", err)
	}
}

func TestServiceUpdateConfigNonStringSecretRejectedNothingPersisted(t *testing.T) {
	svc, fsStore, vault := newTestServiceWithVault(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")

	// webhook_key is declared type string + format password; submit a number.
	err := svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{
		"github_token": "ghp_x", "webhook_key": float64(42),
	})
	if !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("error = %v, want ErrConfigInvalid", err)
	}
	stored, err := fsStore.GetConfig("kandev-plugin-github")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("rejected config must not persist, got %v", stored)
	}
	if _, ok := vault.get(pluginConfigSecretID("kandev-plugin-github", "github_token")); ok {
		t.Fatalf("rejected config must not reach the vault")
	}
}

// failingVault wraps fakeSecretRevealer to inject failures for the
// fail-closed uninstall and delete-after-commit paths.
type failingVault struct {
	*fakeSecretRevealer
	listErr error
}

func (v *failingVault) ListIDs(ctx context.Context) ([]string, error) {
	if v.listErr != nil {
		return nil, v.listErr
	}
	return v.fakeSecretRevealer.ListIDs(ctx)
}

func TestServiceUninstallFailsClosedWhenSecretCleanupFails(t *testing.T) {
	svc, _, rt := newTestService(t)
	vault := &failingVault{fakeSecretRevealer: newFakeSecretRevealer(), listErr: errors.New("vault down")}
	svc.SetSecrets(vault)
	rec := installConfigPlugin(t, svc, "kandev-plugin-github")

	err := svc.Uninstall(context.Background(), "kandev-plugin-github")
	if err == nil {
		t.Fatalf("Uninstall must fail when secret cleanup cannot run")
	}
	// The process is stopped BEFORE the vault purge, so the plugin can't race
	// the cleanup by writing a fresh secret — even on the failure path.
	if !rt.stopped("kandev-plugin-github") {
		t.Fatalf("plugin must be stopped before the vault purge, even when the purge fails")
	}
	// Since the process was stopped, the persisted status must reflect that
	// (error) rather than lie that the plugin is still active.
	stoppedRec, getErr := svc.Get("kandev-plugin-github")
	if getErr != nil {
		t.Fatalf("record should survive a failed uninstall, got %v", getErr)
	}
	if stoppedRec.Status != StatusError {
		t.Fatalf("status = %q, want error after an aborted uninstall stopped the process", stoppedRec.Status)
	}
	if _, statErr := os.Stat(rec.InstallPath); statErr != nil {
		t.Fatalf("package dir should survive a failed uninstall, got %v", statErr)
	}

	// Vault recovers -> retry succeeds.
	vault.listErr = nil
	if err := svc.Uninstall(context.Background(), "kandev-plugin-github"); err != nil {
		t.Fatalf("retry after vault recovery should succeed: %v", err)
	}
}

// failingStore wraps store.Store to fail SetConfig, proving vault entries
// referenced by the still-current config survive a failed commit.
type failingStore struct {
	store.Store
	setConfigErr error
}

func (s *failingStore) SetConfig(id string, config map[string]any) error {
	if s.setConfigErr != nil {
		return s.setConfigErr
	}
	return s.Store.SetConfig(id, config)
}

func TestServiceUpdateConfigFailedCommitKeepsReferencedVaultEntry(t *testing.T) {
	svc, fsStore, vault := newTestServiceWithVault(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")
	ctx := context.Background()

	// Store token + optional webhook_key secret.
	if err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{
		"github_token": "ghp_x", "webhook_key": "whsec_1",
	}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	// Now remove webhook_key, but make the config commit fail.
	failing := &failingStore{Store: fsStore, setConfigErr: errors.New("disk full")}
	svc.store = failing
	err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{
		"github_token": configSecretMask,
	})
	if err == nil {
		t.Fatalf("UpdateConfig should surface the commit failure")
	}
	// The still-current config references webhook_key's vault entry — it
	// must NOT have been deleted (delete happens only after a successful
	// commit).
	if _, ok := vault.get(pluginConfigSecretID("kandev-plugin-github", "webhook_key")); !ok {
		t.Fatalf("vault entry referenced by the current config must survive a failed commit")
	}

	// Commit works again -> removal now deletes the entry.
	svc.store = fsStore
	if err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{
		"github_token": configSecretMask,
	}); err != nil {
		t.Fatalf("UpdateConfig retry: %v", err)
	}
	if _, ok := vault.get(pluginConfigSecretID("kandev-plugin-github", "webhook_key")); ok {
		t.Fatalf("vault entry should be deleted after the successful commit")
	}
}

func TestServiceUpdateConfigFailedCommitRollsBackOverwrittenSecret(t *testing.T) {
	svc, fsStore, vault := newTestServiceWithVault(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")
	ctx := context.Background()

	if err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{"github_token": "ghp_old"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	vaultID := pluginConfigSecretID("kandev-plugin-github", "github_token")

	// Overwrite the token with a new value, but make the config commit fail.
	svc.store = &failingStore{Store: fsStore, setConfigErr: errors.New("disk full")}
	err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{"github_token": "ghp_new"})
	if err == nil {
		t.Fatalf("UpdateConfig should surface the commit failure")
	}
	// The config file was never rewritten, so the vault must resolve to the
	// OLD value — a failed request must not change effective config.
	if v, _ := vault.get(vaultID); v != "ghp_old" {
		t.Fatalf("vault value = %q, want rolled-back ghp_old (a failed commit must not change effective config)", v)
	}
}

func TestServiceUpdateConfigFailedCommitRollsBackNewlyCreatedSecret(t *testing.T) {
	svc, fsStore, vault := newTestServiceWithVault(t)
	installConfigPlugin(t, svc, "kandev-plugin-github")
	ctx := context.Background()

	if err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{"github_token": "ghp_x"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	// Add a brand-new optional secret (no prior vault entry) with a failing commit.
	svc.store = &failingStore{Store: fsStore, setConfigErr: errors.New("disk full")}
	err := svc.UpdateConfig(ctx, "kandev-plugin-github", map[string]any{
		"github_token": configSecretMask, "webhook_key": "whsec_new",
	})
	if err == nil {
		t.Fatalf("UpdateConfig should surface the commit failure")
	}
	// The newly-created vault entry had no prior value, so rollback deletes
	// it — no orphan left behind by a failed request.
	if _, ok := vault.get(pluginConfigSecretID("kandev-plugin-github", "webhook_key")); ok {
		t.Fatalf("newly-created secret must be rolled back (deleted) on a failed commit")
	}
}

func TestServiceUpdateConfigFailsClosedWithoutVault(t *testing.T) {
	svc, fsStore, _ := newTestService(t) // no vault wired
	installConfigPlugin(t, svc, "kandev-plugin-github")

	// A plugin declaring a secret field cannot store config without a vault:
	// fail closed rather than persist the secret in cleartext.
	err := svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{"github_token": "ghp_x"})
	if !errors.Is(err, errSecretVaultRequired) {
		t.Fatalf("error = %v, want errSecretVaultRequired", err)
	}
	stored, err := fsStore.GetConfig("kandev-plugin-github")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("nothing must persist when failing closed, got %v", stored)
	}
}

// ctxAwareVault wraps fakeSecretRevealer and honors context cancellation on
// Set/Delete, so a test can prove rollback writes run on a context detached
// from a cancelled request.
type ctxAwareVault struct {
	*fakeSecretRevealer
}

func (v *ctxAwareVault) Set(ctx context.Context, id, name, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return v.fakeSecretRevealer.Set(ctx, id, name, value)
}

func (v *ctxAwareVault) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return v.fakeSecretRevealer.Delete(ctx, id)
}

func TestStoreConfigSecretsRollbackUsesDetachedContext(t *testing.T) {
	svc, _, _ := newTestService(t)
	vault := &ctxAwareVault{fakeSecretRevealer: newFakeSecretRevealer()}
	svc.SetSecrets(vault)
	rec := installConfigPlugin(t, svc, "kandev-plugin-github")
	vaultID := pluginConfigSecretID("kandev-plugin-github", "github_token")
	vault.set(vaultID, "ghp_old")

	// Stage a new value under a context that we then cancel before rollback,
	// simulating the operator's browser closing mid-save.
	ctx, cancel := context.WithCancel(context.Background())
	_, _, rollback, err := svc.storeConfigSecrets(ctx, rec, map[string]any{"github_token": "ghp_new"})
	if err != nil {
		t.Fatalf("storeConfigSecrets: %v", err)
	}
	cancel()

	// Rollback must still restore the prior value despite the cancelled ctx —
	// it runs on a detached context.
	if rbErr := rollback(); rbErr != nil {
		t.Fatalf("rollback should succeed on a detached context, got %v", rbErr)
	}
	if v, _ := vault.get(vaultID); v != "ghp_old" {
		t.Fatalf("vault value = %q, want restored ghp_old", v)
	}
}

// revealErrVault injects a non-not-found Reveal error for one id, to prove
// storeConfigSecrets refuses to write when a prior value cannot be
// determined (rather than risk a rollback that deletes a real secret).
type revealErrVault struct {
	*fakeSecretRevealer
	failRevealID string
}

func (v *revealErrVault) Reveal(ctx context.Context, id string) (string, error) {
	if id == v.failRevealID {
		return "", errors.New("vault backend unavailable")
	}
	return v.fakeSecretRevealer.Reveal(ctx, id)
}

func TestServiceUpdateConfigAbortsWhenPriorSecretUnreadable(t *testing.T) {
	svc, fsStore, _ := newTestService(t)
	vaultID := pluginConfigSecretID("kandev-plugin-github", "github_token")
	vault := &revealErrVault{fakeSecretRevealer: newFakeSecretRevealer(), failRevealID: vaultID}
	svc.SetSecrets(vault)
	installConfigPlugin(t, svc, "kandev-plugin-github")

	err := svc.UpdateConfig(context.Background(), "kandev-plugin-github", map[string]any{"github_token": "ghp_x"})
	if err == nil {
		t.Fatalf("UpdateConfig must abort when the prior secret cannot be read")
	}
	if _, ok := vault.get(vaultID); ok {
		t.Fatalf("no vault write should happen when the snapshot read fails")
	}
	stored, err := fsStore.GetConfig("kandev-plugin-github")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("nothing must persist when the update aborts, got %v", stored)
	}
}
