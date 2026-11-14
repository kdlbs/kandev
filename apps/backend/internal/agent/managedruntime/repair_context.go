package managedruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	// RepairContextSchemaVersion identifies the canonical reusable-repair
	// installation context. A reader rejects any other version.
	RepairContextSchemaVersion = 1
	repairNPMExecutableName    = "npm"
	maxRepairNPMConfigBytes    = 64 << 10
)

// RepairContext is the canonical installation identity for one reusable
// managed npm repair. It contains bounded fingerprints only; raw installer
// environment values, registry URLs, and credentials never enter metadata.
type RepairContext struct {
	SchemaVersion int    `json:"schema_version"`
	OwnerScope    string `json:"owner_scope"`
	// InstallerEnvDigest fingerprints the authorized installer environment
	// after executor and profile precedence. Provenance is owned by the
	// backend; an empty digest disables reuse.
	InstallerEnvDigest string `json:"installer_env_digest"`
	PackageSpec        string `json:"package_spec"`
	Protocol           string `json:"protocol"`
	Platform           string `json:"platform"`
	User               string `json:"user"`
	Home               string `json:"home"`
	NodeFingerprint    string `json:"node_fingerprint"`
	NPMFingerprint     string `json:"npm_fingerprint"`
	EnvDigest          string `json:"env_digest"`
	CacheRoot          string `json:"cache_root"`
	NPMConfigDigest    string `json:"npm_config_digest"`
}

// RepairContextInput carries the trusted inputs a caller supplies when it asks
// a store to complete an installation context with executor-local facts.
type RepairContextInput struct {
	OwnerScope         string
	InstallerEnvDigest string
	// InheritedEnv is the stable agentctl process environment for task
	// launches. Utility probes leave it empty because Env is already the
	// complete sanitized child environment.
	InheritedEnv []string
	PackageSpec  string
	Protocol     string
	Env          []string
}

// InstallerEnvironmentDigest fingerprints one authorized installer
// environment. The caller decides provenance; an absent provenance channel
// must pass an empty digest instead of calling this helper.
func InstallerEnvironmentDigest(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	canonical := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		canonical = append(canonical, key, values[key])
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// RepairOwnerScopeTask binds one task-owned installation to its authenticated
// application owner. The owner is not the execution-host OS user.
func RepairOwnerScopeTask(ownerID string) string {
	return repairOwnerScope("task", ownerID)
}

// RepairOwnerScopeAgentWide binds agent-wide discovery to the authenticated
// application owner that requested it.
func RepairOwnerScopeAgentWide(ownerID string) string {
	return repairOwnerScope("agent-wide", ownerID)
}

// RepairOwnerScopeProfile binds profile and model-option discovery to the
// authenticated owner and the profile identity resolved by the settings layer.
func RepairOwnerScopeProfile(scope string) string {
	return "profile:" + scope
}

func repairOwnerScope(kind, ownerID string) string {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return ""
	}
	return kind + ":" + ownerID
}

func validRepairOwnerScope(scope string) bool {
	if scope == "" || len(scope) > 512 || strings.ContainsAny(scope, "\x00\r\n") {
		return false
	}
	return strings.HasPrefix(scope, "task:") || strings.HasPrefix(scope, "agent-wide:") ||
		strings.HasPrefix(scope, "profile:")
}

func validRepairProtocol(protocol string) bool {
	return protocol == "acp" || protocol == "codex-app-server"
}

// validate rejects contexts that cannot safely identify an installation.
func (c RepairContext) validate() error {
	if err := c.validateIdentity(); err != nil {
		return err
	}
	return c.validateEnvironment()
}

func (c RepairContext) validateIdentity() error {
	if c.SchemaVersion != RepairContextSchemaVersion {
		return errors.New("repair context schema is unsupported")
	}
	if !validRepairOwnerScope(c.OwnerScope) {
		return errors.New("repair context owner scope is invalid")
	}
	if !validRepairHexDigest(c.InstallerEnvDigest) {
		return errors.New("repair context installer environment digest is unavailable")
	}
	if err := ValidateExactPackageSpec(c.PackageSpec); err != nil {
		return err
	}
	if !validRepairProtocol(c.Protocol) {
		return errors.New("repair context protocol is invalid")
	}
	return nil
}

func (c RepairContext) validateEnvironment() error {
	if err := c.validateOwnerEnvironment(); err != nil {
		return err
	}
	return c.validateToolchainEnvironment()
}

func (c RepairContext) validateOwnerEnvironment() error {
	if c.Platform == "" || len(c.Platform) > 128 {
		return errors.New("repair context platform is invalid")
	}
	if c.User == "" || len(c.User) > 128 {
		return errors.New("repair context user is invalid")
	}
	if c.Home == "" || !filepath.IsAbs(c.Home) || len(c.Home) > 4096 {
		return errors.New("repair context home is invalid")
	}
	return nil
}

func (c RepairContext) validateToolchainEnvironment() error {
	if c.NodeFingerprint == "" || c.NPMFingerprint == "" || len(c.NodeFingerprint) > 512 || len(c.NPMFingerprint) > 512 {
		return errors.New("repair context toolchain is invalid")
	}
	if !validRepairHexDigest(c.EnvDigest) {
		return errors.New("repair context environment digest is invalid")
	}
	if c.CacheRoot == "" || !filepath.IsAbs(c.CacheRoot) || len(c.CacheRoot) > 4096 {
		return errors.New("repair context npm cache is invalid")
	}
	if !validRepairHexDigest(c.NPMConfigDigest) {
		return errors.New("repair context npm configuration is invalid")
	}
	return nil
}

func validRepairHexDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

// canonical encodes the context deterministically for HMAC derivation.
func (c RepairContext) canonical() ([]byte, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(c)
}

// digest derives the opaque context key with the store's private HMAC key.
func (c RepairContext) digest(key []byte) (string, error) {
	encoded, err := c.canonical()
	if err != nil {
		return "", err
	}
	if len(key) == 0 {
		return "", errors.New("repair context fingerprint key is unavailable")
	}
	return repairHMACHex(key, encoded)
}

func newRepairContext(input RepairContextInput, local repairLocalEnvironment) (RepairContext, error) {
	context := RepairContext{
		SchemaVersion:      RepairContextSchemaVersion,
		OwnerScope:         input.OwnerScope,
		InstallerEnvDigest: input.InstallerEnvDigest,
		PackageSpec:        input.PackageSpec,
		Protocol:           input.Protocol,
		Platform:           local.platform,
		User:               local.user,
		Home:               local.home,
		NodeFingerprint:    local.node,
		NPMFingerprint:     local.npm,
		EnvDigest:          local.envDigest,
		CacheRoot:          local.cacheRoot,
		NPMConfigDigest:    local.npmConfigDigest,
	}
	if err := context.validate(); err != nil {
		return RepairContext{}, err
	}
	return context, nil
}

// repairLocalEnvironment is the executor-local half of an installation
// context. It is resolved once per process from stable inputs only.
type repairLocalEnvironment struct {
	platform        string
	user            string
	home            string
	node            string
	npm             string
	envDigest       string
	cacheRoot       string
	npmConfigDigest string
}

func resolveRepairLocalEnvironment(ctx context.Context, env, inheritedEnv []string) (repairLocalEnvironment, error) {
	local := repairLocalEnvironment{
		platform: runtime.GOOS + "/" + runtime.GOARCH,
		user:     repairLocalUserIdentity(),
	}
	var err error
	if local.node, err = repairToolchainFingerprint(ctx, env, "node"); err != nil {
		return repairLocalEnvironment{}, err
	}
	if local.npm, err = repairToolchainFingerprint(ctx, env, repairNPMExecutableName); err != nil {
		return repairLocalEnvironment{}, err
	}
	home, err := repairHomeDirectory(env)
	if err != nil {
		return repairLocalEnvironment{}, err
	}
	local.home = home
	local.cacheRoot = repairNPMCacheRoot(env, home)
	envSource := env
	if len(inheritedEnv) > 0 {
		envSource = inheritedEnv
	}
	local.envDigest = repairEnvironmentDigest(envSource)
	if local.npmConfigDigest, err = repairNPMConfigDigest(home, env, local.npm); err != nil {
		return repairLocalEnvironment{}, err
	}
	return local, nil
}

func repairNPMExecutable(env []string) string {
	searchPath := repairEnvValue(env, "PATH")
	if searchPath == "" {
		searchPath = os.Getenv("PATH")
	}
	if executable, err := repairFindExecutable(searchPath, repairNPMExecutableName); err == nil {
		return executable
	}
	return repairNPMExecutableName
}

func repairToolchainFingerprint(ctx context.Context, env []string, name string) (string, error) {
	searchPath := repairEnvValue(env, "PATH")
	if searchPath == "" {
		searchPath = os.Getenv("PATH")
	}
	executable, err := repairFindExecutable(searchPath, name)
	if err != nil {
		return "", fmt.Errorf("resolve %s for managed runtime repair context: %w", name, err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
		executable = resolved
	}
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("inspect %s for managed runtime repair context", name)
	}
	fingerprint := fmt.Sprintf("%s|%d|%d", executable, info.Size(), info.ModTime().UnixNano())
	if name == repairNPMExecutableName {
		version, versionErr := repairNPMVersion(ctx, executable, env)
		if versionErr != nil {
			return "", versionErr
		}
		fingerprint += "|" + version
	}
	return fingerprint, nil
}

func repairNPMVersion(parent context.Context, executable string, env []string) (string, error) {
	if err := parent.Err(); err != nil {
		return "", err
	}
	key, err := repairNPMVersionFactKey(executable, env)
	if err != nil {
		return "", fmt.Errorf("resolve npm version for managed runtime repair context: %w", err)
	}
	if version, ok := readRepairNPMVersionFact(key); ok {
		return version, nil
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "--version")
	command.Env = append(append([]string(nil), env...),
		"NPM_CONFIG_UPDATE_NOTIFIER=false",
		"npm_config_update_notifier=false",
		"NPM_CONFIG_OFFLINE=true",
		"npm_config_offline=true",
	)
	output, err := command.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("resolve npm version for managed runtime repair context: %w", err)
	}
	version := strings.TrimSpace(string(output))
	if version == "" || len(version) > 128 || strings.ContainsAny(version, "\x00\r\n") {
		return "", errors.New("npm version for managed runtime repair context is invalid")
	}
	writeRepairNPMVersionFact(key, version)
	return version, nil
}

func repairHomeDirectory(env []string) (string, error) {
	home := repairEnvValue(env, "HOME")
	if home == "" {
		if runtime.GOOS == preparedRuntimeWindowsOS {
			home = repairEnvValue(env, "USERPROFILE")
		}
	}
	if home == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return "", fmt.Errorf("resolve home for managed runtime repair context: %w", err)
		}
	}
	if !filepath.IsAbs(home) {
		return "", errors.New("managed runtime repair home directory is not absolute")
	}
	return filepath.Clean(home), nil
}

func repairNPMCacheRoot(env []string, home string) string {
	for _, key := range []string{"npm_config_cache", "NPM_CONFIG_CACHE"} {
		if value := repairEnvValue(env, key); value != "" && filepath.IsAbs(value) {
			return filepath.Clean(value)
		}
	}
	if runtime.GOOS == preparedRuntimeWindowsOS {
		if localAppData := repairEnvValue(env, "LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "npm-cache")
		}
	}
	return filepath.Join(home, ".npm")
}

func repairEnvValue(env []string, key string) string {
	for index := len(env) - 1; index >= 0; index-- {
		name, value, ok := strings.Cut(env[index], "=")
		if !ok {
			continue
		}
		if repairEnvKeyEqual(name, key) {
			return value
		}
	}
	return ""
}

func repairEnvKeyEqual(left, right string) bool {
	if runtime.GOOS == preparedRuntimeWindowsOS {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func repairEnvironmentDigest(env []string) string {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || repairEphemeralEnvironmentKey(key) {
			continue
		}
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	digest := sha256.New()
	for _, key := range keys {
		_, _ = digest.Write([]byte(key))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(values[key]))
		_, _ = digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// repairEphemeralEnvironmentKey excludes only namespaces this process or the
// launcher owns. Authorized installer values still arrive through the
// authenticated provenance digest, so no user input is guessed away here.
// InstallerEnvironmentKeyIsGenerated reports whether an environment key is a
// generated Kandev or transport value that must not enter installation
// identity. Authorized user overrides are not filtered.
func InstallerEnvironmentKeyIsGenerated(key string) bool {
	return repairEphemeralEnvironmentKey(key)
}

func repairEphemeralEnvironmentKey(key string) bool {
	if strings.HasPrefix(key, "KANDEV_") || strings.HasPrefix(key, "AGENTCTL_") ||
		strings.HasPrefix(key, "SSH_") || strings.HasPrefix(key, "GIT_CONFIG_") {
		return true
	}
	switch key {
	case "PORT", "PWD", "OLDPWD", "SHLVL", "_":
		return true
	default:
		return false
	}
}

func repairNPMConfigDigest(home string, env []string, npmFingerprint string) (string, error) {
	userConfigs := []string{
		filepath.Join(home, ".npmrc"),
		filepath.Join(home, ".config", "npm", "npmrc"),
	}
	if path := repairNPMConfigPath(env, "NPM_CONFIG_USERCONFIG"); path != "" {
		userConfigs = append(userConfigs, path)
	}
	paths := append([]string(nil), userConfigs...)
	for _, path := range userConfigs {
		if globalConfig := repairUserGlobalConfigPath(path); globalConfig != "" {
			paths = append(paths, globalConfig)
		}
	}
	if path := repairNPMConfigPath(env, "NPM_CONFIG_GLOBALCONFIG"); path != "" {
		paths = append(paths, path)
	}
	if prefix := repairNPMConfigPath(env, "NPM_CONFIG_PREFIX"); prefix != "" {
		paths = append(paths, filepath.Join(prefix, "etc", "npmrc"))
	}
	if npmExecutable := strings.SplitN(npmFingerprint, "|", 2)[0]; filepath.IsAbs(npmExecutable) {
		if globalConfig := repairNPMGlobalConfigPath(npmExecutable); globalConfig != "" {
			paths = append(paths, globalConfig)
		}
	}
	digest := sha256.New()
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if _, duplicate := seen[path]; duplicate {
			continue
		}
		seen[path] = struct{}{}
		if err := writeBoundedRepairFileDigest(digest, path); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func repairNPMConfigPath(env []string, key string) string {
	for _, candidate := range []string{key, strings.ToLower(key)} {
		value := repairEnvValue(env, candidate)
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			return ""
		}
		return filepath.Clean(value)
	}
	return ""
}

// repairNPMGlobalConfigPath derives npm's effective global npmrc from the
// resolved executable. npm's default is <prefix>/etc/npmrc; a resolved
// .../lib/node_modules/npm/... script must trim the npm package path, not use
// the package's own etc directory.
func repairNPMGlobalConfigPath(executable string) string {
	clean := filepath.Clean(executable)
	slash := filepath.ToSlash(clean)
	if index := strings.Index(slash, "/lib/node_modules/npm/"); index >= 0 {
		return filepath.Join(filepath.FromSlash(slash[:index]), "etc", "npmrc")
	}
	base := filepath.Base(clean)
	if base == "npm" || base == "npm.cmd" {
		return filepath.Join(filepath.Dir(filepath.Dir(clean)), "etc", "npmrc")
	}
	return ""
}

func repairUserGlobalConfigPath(path string) string {
	contents, err := readBoundedRepairConfigFile(path)
	if err != nil {
		return ""
	}
	globalConfig := ""
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "globalconfig") {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if filepath.IsAbs(value) {
			globalConfig = filepath.Clean(value)
		}
	}
	return globalConfig
}

func readBoundedRepairConfigFile(path string) ([]byte, error) {
	file, err := openRepairConfigFile(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxRepairNPMConfigBytes {
		return nil, errors.New("npm configuration for managed runtime repair is not a bounded regular file")
	}
	contents, err := io.ReadAll(io.LimitReader(file, maxRepairNPMConfigBytes+1))
	if err != nil || len(contents) > maxRepairNPMConfigBytes {
		return nil, errors.New("npm configuration for managed runtime repair is unreadable")
	}
	return contents, nil
}

func writeBoundedRepairFileDigest(digest interface{ Write([]byte) (int, error) }, path string) error {
	contents, err := readBoundedRepairConfigFile(path)
	if errors.Is(err, os.ErrNotExist) {
		_, _ = digest.Write([]byte("\x00missing\x00"))
		return nil
	}
	if err != nil {
		return fmt.Errorf("read npm configuration for managed runtime repair: %w", err)
	}
	_, _ = digest.Write([]byte(path))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(contents)
	_, _ = digest.Write([]byte{0})
	return nil
}
