package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	goruntime "runtime"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/internal/plugins/store"
)

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
	if err := validateConfigSchema("test-plugin", map[string]any{"level": float64(2)}, schema); err != nil {
		t.Fatalf("numeric enum should accept float64(2) against int enum: %v", err)
	}
	if err := validateConfigSchema("test-plugin", map[string]any{"level": float64(9)}, schema); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("out-of-enum numeric should still be rejected, got %v", err)
	}
}

func TestValidateConfigSchemaSkipsOwnVaultRefForSecretEnumField(t *testing.T) {
	const pluginID = "kandev-plugin-github"
	// A field that is BOTH secret and enum: on an unchanged save it carries
	// its vault ref, which is none of the enum values.
	schema := map[string]any{
		"properties": map[string]any{
			"env": map[string]any{"type": "string", "secret": true, "enum": []any{"dev", "ops"}},
		},
	}

	// The field's own vault ref (an internal marker) must validate — it was
	// checked when the cleartext was first set.
	if err := validateConfigSchema(pluginID, map[string]any{"env": configVaultRef(pluginID, "env")}, schema); err != nil {
		t.Fatalf("own vault ref for a secret+enum field should validate, got %v", err)
	}
	// A freshly-entered value is still validated against the enum.
	if err := validateConfigSchema(pluginID, map[string]any{"env": "dev"}, schema); err != nil {
		t.Fatalf("valid enum value should pass, got %v", err)
	}
	if err := validateConfigSchema(pluginID, map[string]any{"env": "prod"}, schema); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("out-of-enum value should still be rejected, got %v", err)
	}
	// A ref belonging to a DIFFERENT plugin is not this field's ref, so it is
	// not skipped and fails the enum check.
	if err := validateConfigSchema(pluginID, map[string]any{"env": configVaultRef("other", "env")}, schema); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("a foreign vault ref must not bypass validation, got %v", err)
	}
}

// testPackageWithSecretEnumSchema builds a package whose manifest declares a
// field that is both secret and enum — the combination Greptile flagged as
// 400-ing on every unchanged save.
func testPackageWithSecretEnumSchema(t *testing.T, id string) *bytes.Buffer {
	t.Helper()
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: 1.0.0
display_name: Secret Enum Plugin
capabilities:
  state: true
config_schema:
  type: object
  required: ["env"]
  properties:
    env:
      type: string
      secret: true
      enum: ["dev", "ops"]
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

func TestServiceUpdateConfigUnchangedSecretEnumFieldSaves(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.SetSecrets(newFakeSecretRevealer())
	if _, err := svc.Install(context.Background(), testPackageWithSecretEnumSchema(t, "kandev-plugin-se")); err != nil {
		t.Fatalf("Install: %v", err)
	}
	ctx := context.Background()

	// First save with a valid enum value.
	if err := svc.UpdateConfig(ctx, "kandev-plugin-se", map[string]any{"env": "dev"}); err != nil {
		t.Fatalf("initial UpdateConfig: %v", err)
	}
	// Re-save unchanged (mask): the stored value is now a vault ref, which must
	// not 400 on the enum check.
	if err := svc.UpdateConfig(ctx, "kandev-plugin-se", map[string]any{"env": configSecretMask}); err != nil {
		t.Fatalf("unchanged re-save of a secret+enum field must succeed, got %v", err)
	}
	// Changing to an out-of-enum value is still rejected.
	if err := svc.UpdateConfig(ctx, "kandev-plugin-se", map[string]any{"env": "prod"}); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("out-of-enum value should be rejected, got %v", err)
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
	if err := validateConfigSchema("test-plugin", map[string]any{"pin": float64(1234)}, schema); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("non-string secret must be rejected, got %v", err)
	}
	// Absent stays allowed (an explicit null is already rejected by the
	// property type check — clients omit unset keys).
	if err := validateConfigSchema("test-plugin", map[string]any{}, schema); err != nil {
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
