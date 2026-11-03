//go:build copilot_native

package copilotplugins_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/acpdbg"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

const (
	nativeVersionEnv       = "KANDEV_COPILOT_NATIVE_VERSION"
	nativeArtifactsEnv     = "KANDEV_COPILOT_NATIVE_ARTIFACT_DIR"
	nativePolicyFixtureEnv = "KANDEV_COPILOT_NATIVE_POLICY_FIXTURE"

	nativeTargetPlugin     = "wo03-plugin-target"
	nativeSiblingPlugin    = "wo03-plugin-sibling"
	nativeTargetServer     = "wo03_plugin_target_server"
	nativeSiblingServer    = "wo03_plugin_sibling_server"
	nativeConfigServer     = "wo03_native_config"
	nativeACPServer        = "wo03_acp_injected"
	nativeSentinelToolName = "acpdbg_sentinel"
)

type nativeRunPaths struct {
	artifactRoot string
	runRoot      string
	captureRoot  string
	runtimeRoot  string
	tempRoot     string
	logRoot      string
	workspace    string
	npmPrefix    string
	npmCache     string
}

type nativePluginFixture struct {
	targetRoot  string
	siblingRoot string
}

type nativeCommandResult struct {
	exitCode int
	timedOut bool
	startErr error
	stdout   []byte
	stderr   []byte
}

type nativePolicyFixture struct {
	Source        string `json:"source"`
	FixtureID     string `json:"fixture_id"`
	PolicyKey     string `json:"policy_key"`
	PolicyMatcher string `json:"policy_matcher"`
	TargetServer  string `json:"target_server"`
	AllowedServer string `json:"allowed_server"`
	Disposable    bool   `json:"disposable"`
}

func TestCopilotPluginNativeAcceptance(t *testing.T) {
	version := strings.TrimSpace(os.Getenv(nativeVersionEnv))
	if version != "1.0.88" && version != "1.0.104" {
		t.Fatalf("%s must select exactly 1.0.88 or 1.0.104", nativeVersionEnv)
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatalf("native qualification is scoped to darwin/arm64; got %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	paths, err := prepareNativeRunPaths(version, os.Getenv(nativeArtifactsEnv))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(paths.runtimeRoot); err != nil {
			t.Errorf("remove private runtime fixtures: %v", err)
		}
	})

	env, err := nativeCommandEnvironment(paths)
	if err != nil {
		t.Fatal(err)
	}
	spec := agents.NewCopilotACP().ManagedNPMRuntime()
	if spec.PackageSpec(version) != "@github/copilot@"+version {
		t.Fatalf("managed runtime selected unexpected package for %s", version)
	}
	baseArgs, err := exactCopilotArgs(spec, version, paths.npmPrefix)
	if err != nil {
		t.Fatal(err)
	}

	identityCtx, identityCancel := context.WithTimeout(context.Background(), 90*time.Second)
	identityArgs := append(append([]string(nil), baseArgs...), nativeGlobalArgs(version, paths)...)
	identityArgs = append(identityArgs, "--version")
	identity := runNativeCommand(
		identityCtx,
		identityArgs,
		paths.workspace,
		env,
		filepath.Join(paths.captureRoot, "version.stdout"),
		filepath.Join(paths.captureRoot, "version.stderr"),
	)
	identityCancel()
	metadataVersion, metadataOK := nativePackageMetadataVersion(paths, version)
	if metadataOK {
		t.Logf("case=package-metadata status=PASS version=%s", metadataVersion)
	} else {
		t.Errorf("case=package-metadata status=FAIL reason=exact package metadata was not found or did not match")
	}
	binaryVersionOK := identity.startErr == nil && identity.exitCode == 0 && containsExactVersion(identity.stdout, version)
	if binaryVersionOK {
		t.Logf("case=package-binary-identity status=PASS version=%s exit=%d", version, identity.exitCode)
	} else {
		t.Errorf("case=package-binary-identity status=FAIL exit=%d timed_out=%t", identity.exitCode, identity.timedOut)
	}
	identityOK := metadataOK && binaryVersionOK

	fixtures, err := createNativePluginFixtures(paths.runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	inventoryCtx, inventoryCancel := context.WithTimeout(context.Background(), 90*time.Second)
	inventoryArgs := append(append([]string(nil), baseArgs...), nativeGlobalArgs(version, paths)...)
	inventoryArgs = append(inventoryArgs,
		"--plugin-dir", fixtures.targetRoot,
		"--plugin-dir", fixtures.siblingRoot,
		"plugin", "list", "--json",
	)
	inventory := runNativeCommand(
		inventoryCtx,
		inventoryArgs,
		paths.workspace,
		env,
		filepath.Join(paths.captureRoot, "plugin-list.stdout"),
		filepath.Join(paths.captureRoot, "plugin-list.stderr"),
	)
	inventoryCancel()
	pluginNames := nativePluginNames(inventory.stdout)
	inventoryOK := inventory.startErr == nil && inventory.exitCode == 0 &&
		pluginNames[nativeTargetPlugin] && pluginNames[nativeSiblingPlugin]
	if inventoryOK {
		t.Logf("case=repeated-plugin-dir-paths status=PASS roots=2 target_found=true sibling_found=true exit=%d", inventory.exitCode)
	} else {
		t.Errorf("case=repeated-plugin-dir-paths status=FAIL target_found=%t sibling_found=%t exit=%d timed_out=%t started=%t",
			pluginNames[nativeTargetPlugin], pluginNames[nativeSiblingPlugin], inventory.exitCode, inventory.timedOut, inventory.startErr == nil)
	}

	var positive nativeACPCaseResult
	if identityOK {
		positiveCtx, positiveCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		positive = runNativeCoexistenceCase(
			positiveCtx,
			version,
			baseArgs,
			env,
			paths,
			fixtures,
		)
		positiveCancel()
	} else {
		positive.reason = "not attempted because the exact package identity check failed"
	}
	if !identityOK {
		t.Logf("case=authenticated-acp-mcp-coexistence status=NOT_RUN reason=%s", positive.reason)
		t.Errorf("authenticated ACP coexistence is not qualified because exact package identity failed")
	} else if positive.passed {
		t.Logf("case=authenticated-acp-mcp-coexistence status=PASS evidence=%s",
			nativeACPCaseEvidenceSummary(positive, []string{
				nativeTargetServer,
				nativeSiblingServer,
				nativeConfigServer,
				nativeACPServer,
			}))
	} else {
		t.Errorf("case=authenticated-acp-mcp-coexistence status=FAIL reason=%s evidence=%s",
			positive.reason,
			nativeACPCaseEvidenceSummary(positive, []string{
				nativeTargetServer,
				nativeSiblingServer,
				nativeConfigServer,
				nativeACPServer,
			}))
	}

	if positive.passed && inventoryOK {
		disableCtx, disableCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		disabled := runNativeDisableCase(
			disableCtx,
			version,
			baseArgs,
			env,
			paths,
			fixtures,
		)
		disableCancel()
		if disabled.passed {
			t.Logf("case=native-mcp-disable status=PASS evidence=%s",
				nativeACPCaseEvidenceSummary(disabled, []string{nativeTargetServer, nativeSiblingServer}))
		} else {
			t.Errorf("case=native-mcp-disable status=FAIL reason=%s evidence=%s",
				disabled.reason,
				nativeACPCaseEvidenceSummary(disabled, []string{nativeTargetServer, nativeSiblingServer}))
		}
	} else {
		t.Errorf("case=native-mcp-disable status=NOT_RUN reason=otherwise-working authenticated target and sibling controls were unavailable")
	}

	policy, policyErr := loadNativePolicyFixture(os.Getenv(nativePolicyFixtureEnv), paths.artifactRoot)
	if policyErr != nil {
		t.Logf("case=managed-policy-prohibition status=NOT_RUN reason=%s", policyErr)
		t.Errorf("managed policy qualification cannot proceed without an isolated native policy source")
		return
	}
	if !identityOK || !inventoryOK || !positive.passed {
		t.Log("case=managed-policy-prohibition status=NOT_RUN reason=otherwise-working exact-version plugin controls were unavailable")
		t.Errorf("managed policy qualification cannot proceed without otherwise-working exact-version plugin controls")
		return
	}
	policyCtx, policyCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	policyResult := runNativeManagedPolicyCase(
		policyCtx,
		version,
		baseArgs,
		env,
		paths,
		fixtures,
		policy,
	)
	policyCancel()
	if policyResult.passed {
		t.Logf("case=managed-policy-prohibition status=PASS source=%s policy_key=%s policy_matcher=%s fixture=%s evidence=%s",
			policy.Source,
			policy.PolicyKey,
			policy.PolicyMatcher,
			policy.FixtureID,
			nativeACPCaseEvidenceSummary(policyResult, []string{policy.TargetServer, policy.AllowedServer}))
	} else {
		t.Errorf("case=managed-policy-prohibition status=FAIL reason=%s evidence=%s",
			policyResult.reason,
			nativeACPCaseEvidenceSummary(policyResult, []string{policy.TargetServer, policy.AllowedServer}))
	}
}

type nativeACPCaseResult struct {
	passed                bool
	reason                string
	traffic               map[string]nativeSentinelTrafficSummary
	toolCalls             map[string]int
	successfulCalls       map[string]bool
	nativeDenialObserved  bool
	harnessRejected       bool
	unexpectedToolUpdates int
	captureFailed         bool
}

func prepareNativeRunPaths(version, artifactRoot string) (nativeRunPaths, error) {
	if !filepath.IsAbs(artifactRoot) {
		return nativeRunPaths{}, fmt.Errorf("%s must be an absolute private session-artifact directory", nativeArtifactsEnv)
	}
	if err := os.MkdirAll(artifactRoot, 0o700); err != nil {
		return nativeRunPaths{}, fmt.Errorf("create private native artifact root: %w", err)
	}
	artifactInfo, err := os.Stat(artifactRoot)
	if err != nil || !artifactInfo.IsDir() || artifactInfo.Mode().Perm()&0o077 != 0 {
		return nativeRunPaths{}, errors.New("native artifact directory must be private to the current user")
	}
	runParent := filepath.Join(artifactRoot, "acceptance", version)
	if err := os.MkdirAll(runParent, 0o700); err != nil {
		return nativeRunPaths{}, fmt.Errorf("create private native run parent: %w", err)
	}
	runRoot, err := createUniqueDirectory(runParent)
	if err != nil {
		return nativeRunPaths{}, err
	}
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			_ = os.RemoveAll(runRoot)
		}
	}()

	paths := nativeRunPaths{
		artifactRoot: artifactRoot,
		runRoot:      runRoot,
		captureRoot:  filepath.Join(runRoot, "captures"),
		runtimeRoot:  filepath.Join(runRoot, "runtime"),
	}
	paths.npmCache = filepath.Join(paths.runtimeRoot, "npm-cache")
	paths.tempRoot = filepath.Join(paths.runtimeRoot, "tmp")
	paths.logRoot = filepath.Join(paths.runtimeRoot, "native-logs")
	paths.workspace = filepath.Join(paths.runtimeRoot, "workspace")
	paths.npmPrefix = filepath.Join(paths.runtimeRoot, "npm-project")
	for _, path := range []string{
		paths.captureRoot,
		paths.runtimeRoot,
		paths.tempRoot,
		paths.logRoot,
		paths.workspace,
		paths.npmPrefix,
		paths.npmCache,
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nativeRunPaths{}, fmt.Errorf("create private native fixture directory: %w", err)
		}
	}
	cleanupOnError = false
	return paths, nil
}

func createUniqueDirectory(parent string) (string, error) {
	for range 8 {
		random, err := randomHex(16)
		if err != nil {
			return "", fmt.Errorf("create native run identifier: %w", err)
		}
		path := filepath.Join(parent, "run-"+random)
		if err := os.Mkdir(path, 0o700); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("create private native run directory: %w", err)
		}
	}
	return "", errors.New("could not create a unique private native run directory")
}

func randomHex(byteCount int) (string, error) {
	random := make([]byte, byteCount)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
}

func nativeCommandEnvironment(paths nativeRunPaths) ([]string, error) {
	userConfig := filepath.Join(paths.runtimeRoot, "npm-user.npmrc")
	globalConfig := filepath.Join(paths.runtimeRoot, "npm-global.npmrc")
	for _, path := range []string{userConfig, globalConfig} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			return nil, fmt.Errorf("create private npm configuration: %w", err)
		}
	}

	values := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		lowerKey := strings.ToLower(key)
		switch lowerKey {
		case "copilot_github_token",
			"copilot_allow_all",
			"copilot_assisted_approval",
			"copilot_offline",
			"copilot_cli_version",
			"copilot_cli_dist_dir",
			"gh_token",
			"github_token",
			"npm_token",
			"node_auth_token":
			continue
		}
		if strings.HasPrefix(lowerKey, "npm_config_") {
			continue
		}
		if strings.HasPrefix(lowerKey, "copilot_provider_") ||
			strings.HasPrefix(lowerKey, "copilot_otel_") ||
			strings.HasPrefix(lowerKey, "copilot_custom_instructions") ||
			strings.HasPrefix(lowerKey, "otel_") {
			continue
		}
		if !nativeEnvironmentKeyAllowed(lowerKey) {
			continue
		}
		values[key] = value
	}
	for key, value := range map[string]string{
		"COPILOT_AUTO_UPDATE":     "false",
		"COPILOT_OTEL_ENABLED":    "false",
		"NPM_CONFIG_CACHE":        paths.npmCache,
		"NPM_CONFIG_USERCONFIG":   userConfig,
		"NPM_CONFIG_GLOBALCONFIG": globalConfig,
		"TMPDIR":                  paths.tempRoot,
		"NO_COLOR":                "1",
	} {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env, nil
}

func nativeEnvironmentKeyAllowed(key string) bool {
	key = strings.ToLower(key)
	switch key {
	case "path", "home", "user", "logname", "shell", "lang", "term",
		"term_program", "colorterm", "copilot_home", "gh_host", "copilot_gh_host",
		"http_proxy", "https_proxy", "all_proxy", "no_proxy",
		"ssl_cert_file", "ssl_cert_dir", "node_extra_ca_certs":
		return true
	default:
		return strings.HasPrefix(key, "lc_")
	}
}

func nativeGlobalArgs(version string, paths nativeRunPaths) []string {
	args := []string{
		"--no-auto-update",
		"--no-custom-instructions",
		"--log-dir", paths.logRoot,
	}
	if version == "1.0.104" {
		args = append(args, "--disable-client-telemetry", "--sync-session=off")
	} else {
		args = append(args, "--no-remote", "--no-remote-export")
	}
	return args
}

func exactCopilotArgs(spec agents.ManagedNPMRuntimeSpec, version, npmPrefix string) ([]string, error) {
	args := spec.InteractiveCommand(version).Args()
	replacements := 0
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--prefix" && args[index+1] == managedruntime.NPMProjectPrefix {
			args[index+1] = npmPrefix
			replacements++
		}
	}
	if replacements != 1 {
		return nil, errors.New("managed runtime command did not contain exactly one trusted npm prefix")
	}
	return args, nil
}

func runNativeCommand(
	ctx context.Context,
	args []string,
	workdir string,
	env []string,
	stdoutPath string,
	stderrPath string,
) nativeCommandResult {
	result := nativeCommandResult{exitCode: -1}
	if len(args) == 0 {
		result.startErr = errors.New("native command is empty")
		return result
	}
	invocationPath := strings.TrimSuffix(stdoutPath, filepath.Ext(stdoutPath)) + ".argv.json"
	invocation, err := json.MarshalIndent(map[string]any{
		"args":    args,
		"workdir": workdir,
	}, "", "  ")
	if err != nil {
		result.startErr = err
		return result
	}
	if err := os.WriteFile(invocationPath, invocation, 0o600); err != nil {
		result.startErr = err
		return result
	}
	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		result.startErr = err
		return result
	}
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		_ = stdoutFile.Close()
		result.startErr = err
		return result
	}

	cmd := execCommand(args, workdir, env)
	if err := configureNativeProcessGroup(cmd); err != nil {
		_ = stdoutFile.Close()
		_ = stderrFile.Close()
		result.startErr = err
		return result
	}
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	if err := cmd.Start(); err != nil {
		_ = stdoutFile.Close()
		_ = stderrFile.Close()
		result.startErr = err
		return result
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		select {
		case waitErr = <-done:
		default:
			result.timedOut = true
			_ = killNativeProcessGroup(cmd)
			select {
			case waitErr = <-done:
			case <-time.After(2 * time.Second):
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				waitErr = <-done
			}
		}
	}
	_ = stdoutFile.Close()
	_ = stderrFile.Close()
	if waitErr == nil {
		result.exitCode = 0
	} else {
		var exitError *exec.ExitError
		if errors.As(waitErr, &exitError) {
			result.exitCode = exitError.ExitCode()
		}
	}
	result.stdout, _ = os.ReadFile(stdoutPath)
	result.stderr, _ = os.ReadFile(stderrPath)
	return result
}

func execCommand(args []string, workdir string, env []string) *exec.Cmd {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = workdir
	cmd.Env = env
	return cmd
}

func containsExactVersion(output []byte, version string) bool {
	line := strings.TrimSpace(strings.SplitN(string(output), "\n", 2)[0])
	if strings.HasPrefix(line, "GitHub Copilot CLI ") {
		line = strings.TrimPrefix(line, "GitHub Copilot CLI ")
		line = strings.TrimSuffix(line, ".")
	}
	return line == version
}

func nativePackageMetadataVersion(paths nativeRunPaths, expected string) (string, bool) {
	candidates := []string{
		filepath.Join(paths.npmPrefix, "node_modules", "@github", "copilot", "package.json"),
	}
	cached, err := filepath.Glob(filepath.Join(
		paths.npmCache,
		"_npx",
		"*",
		"node_modules",
		"@github",
		"copilot",
		"package.json",
	))
	if err != nil {
		return "", false
	}
	candidates = append(candidates, cached...)
	versions := make(map[string]bool)
	for _, path := range candidates {
		info, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return "", false
		}
		if !info.Mode().IsRegular() {
			return "", false
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", false
		}
		var metadata struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal(data, &metadata); err != nil {
			return "", false
		}
		if metadata.Name == "@github/copilot" && metadata.Version != "" {
			versions[metadata.Version] = true
		}
	}
	if len(versions) != 1 {
		return "", false
	}
	for version := range versions {
		return version, version == expected
	}
	return "", false
}

func nativePluginNames(output []byte) map[string]bool {
	var rows []map[string]any
	if err := json.Unmarshal(output, &rows); err != nil {
		return map[string]bool{}
	}
	names := make(map[string]bool, len(rows))
	for _, row := range rows {
		if name, ok := row["name"].(string); ok {
			names[name] = true
		}
	}
	return names
}

func createNativePluginFixtures(runtimeRoot string) (nativePluginFixture, error) {
	roots := filepath.Join(runtimeRoot, "plugin roots")
	targetRoot := filepath.Join(roots, "target ;[space] $")
	siblingRoot := filepath.Join(roots, "sibling [control]")
	for _, root := range []string{targetRoot, siblingRoot} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return nativePluginFixture{}, fmt.Errorf("create private plugin root: %w", err)
		}
	}
	fixtures := nativePluginFixture{targetRoot: targetRoot, siblingRoot: siblingRoot}
	if err := writeNativePlugin(fixtures.targetRoot, nativeTargetPlugin, nativeTargetServer); err != nil {
		return nativePluginFixture{}, err
	}
	if err := writeNativePlugin(fixtures.siblingRoot, nativeSiblingPlugin, nativeSiblingServer); err != nil {
		return nativePluginFixture{}, err
	}
	return fixtures, nil
}

func writeNativePlugin(root, name, server string) error {
	manifest, err := json.Marshal(map[string]any{
		"name":       name,
		"version":    "1.0.0",
		"mcpServers": ".mcp.json",
	})
	if err != nil {
		return fmt.Errorf("encode private plugin manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), manifest, 0o600); err != nil {
		return fmt.Errorf("write private plugin manifest: %w", err)
	}
	if err := writePluginMCPConfig(root, server, ""); err != nil {
		return err
	}
	return nil
}

func writePluginMCPConfig(root, serverName, url string) error {
	servers := map[string]any{}
	if url != "" {
		servers[serverName] = map[string]any{
			"type":  "http",
			"url":   url,
			"tools": []string{"*"},
		}
	}
	data, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err != nil {
		return fmt.Errorf("encode private plugin MCP configuration: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mcp.json"), data, 0o600); err != nil {
		return fmt.Errorf("write private plugin MCP configuration: %w", err)
	}
	return nil
}

func loadNativePolicyFixture(path, artifactRoot string) (nativePolicyFixture, error) {
	if strings.TrimSpace(path) == "" {
		return nativePolicyFixture{}, errors.New(
			"no isolated native managed-policy fixture is configured: exact CLI help exposes no per-run policy-source override; official macOS sources are forced MDM preferences, the fixed root-owned /Library/Application Support/GitHubCopilot/managed-settings.json file, or account-bound server policy; no disposable darwin/arm64 policy context is available without changing a real source",
		)
	}
	if !filepath.IsAbs(path) {
		return nativePolicyFixture{}, errors.New("managed-policy fixture selector must name a private absolute fixture descriptor")
	}
	realRoot, err := filepath.EvalSymlinks(artifactRoot)
	if err != nil {
		return nativePolicyFixture{}, errors.New("private native artifact root could not be resolved")
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nativePolicyFixture{}, errors.New("managed-policy fixture descriptor could not be resolved")
	}
	relative, err := filepath.Rel(realRoot, realPath)
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nativePolicyFixture{}, errors.New("managed-policy fixture descriptor must remain inside the private session-artifact directory")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nativePolicyFixture{}, errors.New("managed-policy fixture descriptor must be a regular private file")
	}
	data, err := os.ReadFile(realPath)
	if err != nil {
		return nativePolicyFixture{}, errors.New("managed-policy fixture descriptor could not be read")
	}
	var fixture nativePolicyFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nativePolicyFixture{}, errors.New("managed-policy fixture descriptor is invalid")
	}
	switch fixture.Source {
	case "mdm", "file-based", "server-managed":
	default:
		return nativePolicyFixture{}, errors.New("managed-policy fixture must identify a native MDM, file-based, or server-managed source")
	}
	if !fixture.Disposable ||
		!validNativeFixtureID(fixture.FixtureID) ||
		fixture.PolicyKey != "deniedMcpServers" ||
		fixture.PolicyMatcher != "serverName" ||
		fixture.TargetServer != nativeTargetServer ||
		fixture.AllowedServer != nativeSiblingServer {
		return nativePolicyFixture{}, errors.New("managed-policy fixture must be disposable and name the exact denied-mcp policy, target, and allowed sibling identities")
	}
	return fixture, nil
}

func validNativeFixtureID(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '-' && char != '_' && char != '.' {
			return false
		}
	}
	return true
}

func nativeToolNames(servers ...string) []string {
	names := make([]string, 0, len(servers))
	for _, server := range servers {
		names = append(names, server+"-"+nativeSentinelToolName)
	}
	return names
}

func nativeAdditionalMCPConfig(server, url string) string {
	data, _ := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			server: map[string]any{
				"type":  "http",
				"url":   url,
				"tools": []string{"*"},
			},
		},
	})
	return string(data)
}

func newNativeRecorder(paths nativeRunPaths, name string) (*acpdbg.Recorder, error) {
	return acpdbg.NewRecorder(filepath.Join(paths.captureRoot, name+".jsonl"))
}
