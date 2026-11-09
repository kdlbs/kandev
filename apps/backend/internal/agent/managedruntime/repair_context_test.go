package managedruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRepairContextSeparatesTrustedInputs(t *testing.T) {
	store, home := openTestRepairStore(t)
	base := testRepairContextInput(t, home, "managed-acp@1.2.3")
	repairCtx, err := store.Context(context.Background(), base)
	if err != nil {
		t.Fatalf("complete repair context: %v", err)
	}
	digest, err := repairCtx.digest(store.key)
	if err != nil {
		t.Fatalf("digest repair context: %v", err)
	}
	mutations := map[string]func(*RepairContextInput){
		"installer environment": func(input *RepairContextInput) {
			input.InstallerEnvDigest = InstallerEnvironmentDigest(map[string]string{"PROFILE": "changed"})
		},
		"package version": func(input *RepairContextInput) { input.PackageSpec = "managed-acp@1.2.4" },
		"protocol":        func(input *RepairContextInput) { input.Protocol = "codex-app-server" },
		"scope":           func(input *RepairContextInput) { input.OwnerScope = RepairOwnerScopeAgentWide("other") },
		"home directory":  func(input *RepairContextInput) { input.Env = replaceRepairEnv(input.Env, "HOME", t.TempDir()) },
		"npm cache": func(input *RepairContextInput) {
			input.Env = replaceRepairEnv(input.Env, "NPM_CONFIG_CACHE", t.TempDir())
		},
		"toolchain inputs": func(input *RepairContextInput) {
			input.Env = replaceRepairEnv(input.Env, "PATH", testRepairToolchainBin(t))
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changedInput := base
			mutate(&changedInput)
			changed, contextErr := store.Context(context.Background(), changedInput)
			if contextErr != nil {
				t.Fatalf("complete changed repair context: %v", contextErr)
			}
			changedDigest, digestErr := changed.digest(store.key)
			if digestErr != nil {
				t.Fatalf("digest changed repair context: %v", digestErr)
			}
			if changedDigest == digest {
				t.Fatalf("changed %s reused the installation context digest", name)
			}
		})
	}

	missingProvenance := base
	missingProvenance.InstallerEnvDigest = ""
	if _, err := store.Context(context.Background(), missingProvenance); err == nil {
		t.Fatal("accepted a repair context without installer environment provenance")
	}
	tampered := repairCtx
	tampered.PackageSpec = "managed-acp"
	if _, err := tampered.digest(store.key); err == nil {
		t.Fatal("accepted a tampered canonical context")
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.19
func TestRepairContextCancellation(t *testing.T) {
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Context(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context completion error = %v, want context cancellation", err)
	}
}

func TestRepairContextBoundsAndSecrets(t *testing.T) {
	store, home := openTestRepairStore(t)
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	input.InstallerEnvDigest = InstallerEnvironmentDigest(map[string]string{"NPM_TOKEN": "repair-secret-value"})
	repairCtx, err := store.Context(context.Background(), input)
	if err != nil {
		t.Fatalf("complete repair context: %v", err)
	}
	lease, err := store.BeginRecovery(context.Background(), repairCtx)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	defer func() { _ = lease.Close() }()
	prepared, err := lease.Prepare(context.Background(), testRepairEnv(input.Env), testRepairRunner(t, input.PackageSpec))
	if err != nil {
		t.Fatalf("prepare generation: %v", err)
	}
	if err := prepared.PublishRepair(); err != nil {
		t.Fatalf("publish repair: %v", err)
	}
	err = filepath.Walk(store.RepairRoot(), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if info.Size() > maxRepairNPMConfigBytes {
			return errors.New("repair record exceeds the fixed bound")
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), "repair-secret-value") {
			return errors.New("repair metadata contains a raw installer secret")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if repairCtx.InstallerEnvDigest == "" {
		t.Fatal("installer environment digest is empty")
	}
	if len(repairCtx.InstallerEnvDigest) != sha256.Size*2 {
		t.Fatalf("installer environment digest length = %d", len(repairCtx.InstallerEnvDigest))
	}
}

func TestRepairContextRejectsUnsafeStore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	base := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(base, repairStoreRootName)); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRepairStore(base); err == nil {
		t.Fatal("opened a repair store through a symlinked root")
	}
	if _, err := OpenRepairStore(""); err == nil {
		t.Fatal("opened a repair store without a home directory")
	}
	if _, err := OpenRepairStore(string(filepath.Separator)); err == nil {
		t.Fatal("opened a repair store at the filesystem root")
	}
}

func openTestRepairStore(t *testing.T) (*RepairStore, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("reusable repair requires native ACL verification on Windows")
	}
	home := t.TempDir()
	store, err := OpenRepairStore(home)
	if err != nil {
		t.Fatalf("open repair store: %v", err)
	}
	return store, home
}

func testRepairToolchainBin(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	for name, version := range map[string]string{"node": "24.0.0", "npm": "11.0.0"} {
		script := "#!/bin/sh\necho " + version + "\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

func testRepairContextInput(t *testing.T, home, packageSpec string) RepairContextInput {
	t.Helper()
	bin := testRepairToolchainBin(t)
	return RepairContextInput{
		OwnerScope:         RepairOwnerScopeTask("owner-a"),
		InstallerEnvDigest: InstallerEnvironmentDigest(map[string]string{"PROFILE": "baseline"}),
		PackageSpec:        packageSpec,
		Protocol:           "acp",
		Env: []string{
			"HOME=" + home,
			"PATH=" + bin,
			"NPM_CONFIG_CACHE=" + filepath.Join(t.TempDir(), "npm-cache"),
		},
	}
}

func testRepairEnv(env []string) []string {
	return append([]string(nil), env...)
}

func replaceRepairEnv(env []string, key, value string) []string {
	replaced := make([]string, 0, len(env)+1)
	for _, item := range env {
		name, _, ok := strings.Cut(item, "=")
		if ok && name == key {
			continue
		}
		replaced = append(replaced, item)
	}
	return append(replaced, key+"="+value)
}

func testRepairRunner(t *testing.T, packageSpec string) ManagedRuntimeCommandRunner {
	t.Helper()
	return func(_ context.Context, command ManagedRuntimeCommand) error {
		return populatePreparedRuntimeCache(command.Args, packageSpec)
	}
}

func mustRepairContextDigest(t *testing.T, store *RepairStore, repairContext RepairContext) string {
	t.Helper()
	digest, err := repairContext.digest(store.key)
	if err != nil {
		t.Fatalf("digest repair context: %v", err)
	}
	if _, err := hex.DecodeString(digest); err != nil {
		t.Fatalf("decode context digest: %v", err)
	}
	return digest
}

func TestRepairContextBindsOwnerAndInheritedEnvironment(t *testing.T) {
	store, home := openTestRepairStore(t)
	base := testRepairContextInput(t, home, "managed-acp@1.2.3")
	first, err := store.Context(context.Background(), base)
	if err != nil {
		t.Fatalf("complete first owner context: %v", err)
	}
	secondInput := base
	secondInput.OwnerScope = RepairOwnerScopeTask("owner-b")
	second, err := store.Context(context.Background(), secondInput)
	if err != nil {
		t.Fatalf("complete second owner context: %v", err)
	}
	firstDigest, _ := first.digest(store.key)
	secondDigest, _ := second.digest(store.key)
	if firstDigest == secondDigest {
		t.Fatal("two application owners shared one installation context")
	}

	inheritedChanged := base
	inheritedChanged.InheritedEnv = []string{"HOME=" + home, "NPM_CONFIG_REGISTRY=https://changed.example.test/"}
	changed, err := store.Context(context.Background(), inheritedChanged)
	if err != nil {
		t.Fatalf("complete inherited environment context: %v", err)
	}
	changedDigest, _ := changed.digest(store.key)
	if changedDigest == firstDigest {
		t.Fatal("changed inherited installer environment reused the installation context")
	}
}

func TestRepairContextTracksEffectiveNPMConfigFiles(t *testing.T) {
	store, home := openTestRepairStore(t)
	userConfig := filepath.Join(t.TempDir(), "custom-npmrc")
	globalConfig := filepath.Join(t.TempDir(), "global-npmrc")
	if err := os.WriteFile(userConfig, []byte("registry=https://one.example.test/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalConfig, []byte("audit=false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := testRepairContextInput(t, home, "managed-acp@1.2.3")
	input.Env = append(input.Env,
		"NPM_CONFIG_USERCONFIG="+userConfig,
		"NPM_CONFIG_GLOBALCONFIG="+globalConfig,
	)
	first, err := store.Context(context.Background(), input)
	if err != nil {
		t.Fatalf("complete custom npm config context: %v", err)
	}
	firstDigest, _ := first.digest(store.key)
	if err := os.WriteFile(userConfig, []byte("registry=https://two.example.test/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := store.Context(context.Background(), input)
	if err != nil {
		t.Fatalf("complete changed custom npm config context: %v", err)
	}
	secondDigest, _ := second.digest(store.key)
	if secondDigest == firstDigest {
		t.Fatal("changed custom npmrc content reused the installation context")
	}
	if err := os.WriteFile(globalConfig, []byte("audit=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	third, err := store.Context(context.Background(), input)
	if err != nil {
		t.Fatalf("complete changed global npm config context: %v", err)
	}
	thirdDigest, _ := third.digest(store.key)
	if thirdDigest == secondDigest {
		t.Fatal("changed global npmrc content reused the installation context")
	}
	published := publishTestRepair(t, store, input, first)
	if store.MatchesRepairContext(published, third) {
		t.Fatal("a repair prepared before an npmrc change matched the successor context")
	}
}

func TestRepairNPMGlobalConfigPath(t *testing.T) {
	for executable, want := range map[string]string{
		"/usr/local/bin/npm":                             "/usr/local/etc/npmrc",
		"/usr/local/lib/node_modules/npm/bin/npm-cli.js": "/usr/local/etc/npmrc",
		"/home/user/.nvm/versions/node/v22/bin/npm":      "/home/user/.nvm/versions/node/v22/etc/npmrc",
		"/usr/lib/node_modules/npm/bin/npm-cli.js":       "/usr/etc/npmrc",
	} {
		if got := repairNPMGlobalConfigPath(executable); got != want {
			t.Fatalf("global npmrc for %q = %q, want %q", executable, got, want)
		}
	}
	if got := repairNPMGlobalConfigPath("/usr/local/bin/node"); got != "" {
		t.Fatalf("non-npm executable global npmrc = %q, want empty", got)
	}
}

func TestRepairNPMConfigPathAcceptsLowercaseOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "npmrc")
	for _, key := range []string{"NPM_CONFIG_USERCONFIG", "npm_config_userconfig"} {
		if got := repairNPMConfigPath([]string{key + "=" + path}, "NPM_CONFIG_USERCONFIG"); got != path {
			t.Fatalf("%s path = %q, want %q", key, got, path)
		}
	}
	if got := repairNPMConfigPath([]string{"npm_config_userconfig=relative"}, "NPM_CONFIG_USERCONFIG"); got != "" {
		t.Fatalf("relative npm config path = %q, want empty", got)
	}
}
