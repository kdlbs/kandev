package utility

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"go.uber.org/zap"
)

const (
	runtimeObservationTimeout = 2 * time.Second
	runtimeManifestMaxBytes   = 64 << 10
	runtimeCommandMaxBytes    = 4 << 10
	windowsGOOS               = "windows"
)

var (
	packageNamePattern  = regexp.MustCompile(`^(?:@[A-Za-z0-9._-]+/)?[A-Za-z0-9._-]+$`)
	codexVersionPattern = regexp.MustCompile(`(?m)^codex-cli[ \t]+(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?[ \t]*\r?$`)
)

type boundedObservationBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedObservationBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := runtimeCommandMaxBytes - b.Len()
	if remaining <= 0 {
		b.overflow = true
		return written, nil
	}
	if len(value) > remaining {
		value = value[:remaining]
		b.overflow = true
	}
	_, _ = b.Buffer.Write(value)
	return written, nil
}

func (b *boundedObservationBuffer) ReadFrom(reader io.Reader) (int64, error) {
	buffer := make([]byte, 32<<10)
	var total int64
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			_, _ = b.Write(buffer[:n])
			total += int64(n)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return total, nil
			}
			return total, err
		}
	}
}

func (e *ACPInferenceExecutor) collectRuntimeObservation(
	ctx context.Context,
	descriptor *agents.RuntimeObservationDescriptor,
	command []string,
	commandPrefix []string,
	preparedArgs []string,
	env []string,
	launchedExecutable string,
	workDir string,
	agentVersion string,
) *agents.RuntimeInfo {
	if descriptor == nil {
		return nil
	}
	inspectCtx, cancel := context.WithTimeout(ctx, runtimeObservationTimeout)
	defer cancel()

	info := &agents.RuntimeInfo{
		Scope:      "host",
		ObservedAt: time.Now().UTC(),
		Components: make([]agents.RuntimeComponent, 0, 2),
	}
	defer func() { info.ObservedAt = time.Now().UTC() }()
	bridge := componentFromDescriptor(agents.RuntimeComponentBridge, descriptor.Bridge)
	if len(commandPrefix) > 0 {
		bridge.Source = agents.RuntimeComponentUnknown
		bridge.ObservedVersion = ""
	}
	packageSpec, managedBridge := exactRuntimePackage(command, descriptor.Bridge.Package)
	if len(commandPrefix) == 0 {
		bridge.ObservedVersion = normalizedStableVersion(agentVersion)
	}
	info.Components = append(info.Components, bridge)

	if descriptor.Provider == nil {
		return info
	}
	provider := componentFromDescriptor(agents.RuntimeComponentProvider, *descriptor.Provider)
	if len(commandPrefix) > 0 {
		provider.Source = agents.RuntimeComponentUnknown
		provider.Owner = agents.RuntimeComponentOwnerUnknown
		provider.Package = ""
		provider.GuidanceURL = ""
		info.Components = append(info.Components, provider)
		return info
	}

	provider, externalCodex := observeExternalCodexProvider(
		inspectCtx, *descriptor.Provider, provider, env, workDir, e.logger,
	)
	if externalCodex {
		info.Components = append(info.Components, provider)
		return info
	}

	provider.Source = validComponentSource(provider.Source)
	provider.Owner = validComponentOwner(provider.Owner)
	if provider.Source == agents.RuntimeComponentBundled && managedBridge {
		cacheRoot, err := resolveNPMCacheRoot(
			inspectCtx, launchedExecutable, preparedArgs, env, workDir, e.logger,
		)
		if err == nil {
			provider.ObservedVersion = inspectBundledDependencyVersion(
				cacheRoot, packageSpec, descriptor.Bridge.Package, provider.Package,
			)
		}
	}
	info.Components = append(info.Components, provider)
	return info
}

func observeExternalCodexProvider(
	ctx context.Context,
	descriptor agents.RuntimeComponentDescriptor,
	provider agents.RuntimeComponent,
	env []string,
	workDir string,
	logger *zap.Logger,
) (agents.RuntimeComponent, bool) {
	if descriptor.ExternalVersionEnv != "CODEX_PATH" || descriptor.Package != "@openai/codex" {
		return provider, false
	}
	binary := environmentValue(env, "CODEX_PATH")
	if binary == "" {
		return provider, false
	}
	provider.Source = agents.RuntimeComponentExternal
	provider.Owner = agents.RuntimeComponentOwnerExternal
	executable, ok := resolveExecutableInEnvironment(binary, env, workDir)
	if !ok {
		return provider, true
	}
	output, err := runRuntimeObservationCommand(ctx, executable, []string{"--version"}, env, workDir, logger)
	if err == nil {
		provider.ObservedVersion = parseCodexVersion(output)
	}
	return provider, true
}

func componentFromDescriptor(
	role agents.RuntimeComponentRole,
	descriptor agents.RuntimeComponentDescriptor,
) agents.RuntimeComponent {
	component := agents.RuntimeComponent{
		Role:   role,
		Name:   trustedRuntimeName(descriptor.Name),
		Source: validComponentSource(descriptor.Source),
		Owner:  validComponentOwner(descriptor.Owner),
	}
	if guidanceURL := trustedGuidanceURL(descriptor.GuidanceURL); guidanceURL != "" {
		component.GuidanceURL = guidanceURL
	}
	if validRuntimePackage(descriptor.Package) {
		component.Package = descriptor.Package
	}
	return component
}

func trustedGuidanceURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	return parsed.String()
}

func trustedRuntimeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 || strings.ContainsAny(value, "\x00\r\n") {
		return "Runtime"
	}
	return value
}

func validRuntimePackage(value string) bool {
	if len(value) > 128 || !packageNamePattern.MatchString(value) {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validComponentSource(value agents.RuntimeComponentSource) agents.RuntimeComponentSource {
	switch value {
	case agents.RuntimeComponentManaged, agents.RuntimeComponentBundled, agents.RuntimeComponentExternal:
		return value
	default:
		return agents.RuntimeComponentUnknown
	}
}

func validComponentOwner(value agents.RuntimeComponentOwner) agents.RuntimeComponentOwner {
	switch value {
	case agents.RuntimeComponentOwnerKandev, agents.RuntimeComponentOwnerExternal:
		return value
	default:
		return agents.RuntimeComponentOwnerUnknown
	}
}

func exactRuntimePackage(command []string, trustedPackage string) (string, bool) {
	if !validRuntimePackage(trustedPackage) {
		return "", false
	}
	packageSpec, ok := managedRuntimeProbePackageSpec(command)
	if !ok || !strings.HasPrefix(packageSpec, trustedPackage+"@") {
		return "", false
	}
	if err := managedruntime.ValidateExactPackageSpec(packageSpec); err != nil {
		return "", false
	}
	return packageSpec, true
}

func packageVersion(packageSpec, packageName string) string {
	prefix := packageName + "@"
	if !strings.HasPrefix(packageSpec, prefix) {
		return ""
	}
	return normalizedStableVersion(strings.TrimPrefix(packageSpec, prefix))
}

func normalizedStableVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v")
	if _, err := managedruntime.ParseStableVersion(value); err != nil {
		return ""
	}
	return value
}

func parseCodexVersion(output []byte) string {
	match := codexVersionPattern.FindSubmatch(output)
	if len(match) < 4 {
		return ""
	}
	version := string(match[1]) + "." + string(match[2]) + "." + string(match[3])
	if len(match) > 4 && len(match[4]) > 0 {
		version += string(match[4])
	}
	return normalizedStableVersion(version)
}

func environmentValue(env []string, key string) string {
	result := ""
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if environmentKeyEqual(name, key) {
			result = value
		}
	}
	return result
}

func resolveNPMCacheRoot(
	ctx context.Context,
	launchedExecutable string,
	preparedArgs []string,
	env []string,
	workDir string,
	logger *zap.Logger,
) (string, error) {
	npm, ok := resolveSiblingNPMExecutable(launchedExecutable, env, workDir)
	if !ok {
		return "", errors.New("npm executable beside launched npx is unavailable")
	}
	args := []string{}
	if prefix := managedNPMProjectPrefix(preparedArgs); prefix != "" {
		args = append(args, codexNpmPrefixFlag, prefix)
	}
	args = append(args, "config", "get", "cache")
	output, err := runRuntimeObservationCommand(ctx, npm, args, env, workDir, logger)
	if err != nil {
		return "", err
	}
	cacheRoot := ""
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line != "" {
			cacheRoot = line
		}
	}
	if cacheRoot == "" || !filepath.IsAbs(cacheRoot) {
		return "", errors.New("npm cache root is not absolute")
	}
	return filepath.Clean(cacheRoot), nil
}

func resolveSiblingNPMExecutable(launchedExecutable string, env []string, workDir string) (string, bool) {
	if strings.TrimSpace(launchedExecutable) == "" || strings.ContainsAny(launchedExecutable, "\x00\r\n") {
		return "", false
	}
	if !filepath.IsAbs(launchedExecutable) {
		launchedExecutable = filepath.Join(workDir, launchedExecutable)
	}
	launchedExecutable, err := filepath.Abs(launchedExecutable)
	if err != nil {
		return "", false
	}
	extension := filepath.Ext(launchedExecutable)
	npm := filepath.Join(filepath.Dir(launchedExecutable), "npm"+extension)
	candidates := []string{npm}
	if extension == "" {
		candidates = executableCandidates(npm, env)
	}
	for _, candidate := range candidates {
		if path, ok := executableFile(candidate); ok {
			return path, true
		}
	}
	return "", false
}

func managedNPMProjectPrefix(args []string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == codexNpmPrefixFlag && args[index+1] != managedruntime.NPMProjectPrefix {
			return args[index+1]
		}
	}
	return ""
}

type packageManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type manifestSnapshot struct {
	root     string
	path     string
	realPath string
	info     os.FileInfo
	digest   [32]byte
}

func inspectBundledDependencyVersion(cacheRoot, bridgeSpec, bridgePackage, dependencyPackage string) string {
	bridgeVersion := packageVersion(bridgeSpec, bridgePackage)
	if bridgeVersion == "" || !validRuntimePackage(bridgePackage) || !validRuntimePackage(dependencyPackage) {
		return ""
	}
	realNpxRoot := resolveManagedNPMCacheRoot(cacheRoot, bridgeSpec)
	if realNpxRoot == "" {
		return ""
	}
	return inspectManagedNPMDependency(realNpxRoot, bridgeVersion, bridgePackage, dependencyPackage)
}

func resolveManagedNPMCacheRoot(cacheRoot, bridgeSpec string) string {
	cacheRealRoot, err := filepath.EvalSymlinks(cacheRoot)
	if err != nil {
		return ""
	}
	expectedRoot := filepath.Join(cacheRealRoot, "_npx", managedruntime.NpxExecutionCacheKey(bridgeSpec))
	npxRoot := filepath.Join(cacheRoot, "_npx", managedruntime.NpxExecutionCacheKey(bridgeSpec))
	npxInfo, err := os.Lstat(npxRoot)
	if err != nil || !npxInfo.IsDir() || npxInfo.Mode()&os.ModeSymlink != 0 {
		return ""
	}
	realRoot, err := filepath.EvalSymlinks(npxRoot)
	if err != nil || filepath.Clean(realRoot) != filepath.Clean(expectedRoot) {
		return ""
	}
	rootInfo, err := os.Stat(realRoot)
	if err != nil || !rootInfo.IsDir() {
		return ""
	}
	return realRoot
}

func inspectManagedNPMDependency(
	realRoot, bridgeVersion, bridgePackage, dependencyPackage string,
) string {
	bridgeManifestPath := filepath.Join(realRoot, "node_modules", filepath.FromSlash(bridgePackage), "package.json")
	bridgeManifest, bridgeSnapshot, ok := readRuntimeManifest(realRoot, bridgeManifestPath)
	if !ok || bridgeManifest.Name != bridgePackage || bridgeManifest.Version != bridgeVersion {
		return ""
	}
	dependencyPath, ok := resolveNodeDependencyManifest(realRoot, filepath.Dir(bridgeManifestPath), dependencyPackage)
	if !ok {
		return ""
	}
	dependencyManifest, dependencySnapshot, ok := readRuntimeManifest(realRoot, dependencyPath)
	if !ok || dependencyManifest.Name != dependencyPackage || normalizedStableVersion(dependencyManifest.Version) == "" {
		return ""
	}
	if !runtimeManifestUnchanged(bridgeSnapshot) || !runtimeManifestUnchanged(dependencySnapshot) {
		return ""
	}
	return normalizedStableVersion(dependencyManifest.Version)
}

func resolveNodeDependencyManifest(root, bridgeDir, packageName string) (string, bool) {
	for current := filepath.Clean(bridgeDir); ; current = filepath.Dir(current) {
		candidate := filepath.Join(current, "node_modules", filepath.FromSlash(packageName), "package.json")
		if pathWithin(root, candidate) {
			if _, err := os.Lstat(candidate); err == nil {
				return candidate, true
			}
		}
		if current == root || current == filepath.Dir(current) {
			break
		}
	}
	return "", false
}

func readRuntimeManifest(root, manifestPath string) (packageManifest, manifestSnapshot, bool) {
	var manifest packageManifest
	if !pathWithin(root, manifestPath) {
		return manifest, manifestSnapshot{}, false
	}
	realPath, err := filepath.EvalSymlinks(manifestPath)
	if err != nil || !pathWithin(root, realPath) {
		return manifest, manifestSnapshot{}, false
	}
	file, err := os.Open(realPath)
	if err != nil {
		return manifest, manifestSnapshot{}, false
	}
	defer func() { _ = file.Close() }()
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() > runtimeManifestMaxBytes {
		return manifest, manifestSnapshot{}, false
	}
	data, err := io.ReadAll(io.LimitReader(file, runtimeManifestMaxBytes+1))
	if err != nil || len(data) > runtimeManifestMaxBytes {
		return manifest, manifestSnapshot{}, false
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return manifest, manifestSnapshot{}, false
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, manifestSnapshot{}, false
	}
	return manifest, manifestSnapshot{
		root: root, path: manifestPath, realPath: realPath, info: after, digest: sha256.Sum256(data),
	}, true
}

func runtimeManifestUnchanged(snapshot manifestSnapshot) bool {
	realPath, err := filepath.EvalSymlinks(snapshot.path)
	if err != nil || !pathWithin(snapshot.root, realPath) || realPath != snapshot.realPath {
		return false
	}
	file, err := os.Open(realPath)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	current, err := file.Stat()
	if err != nil || !current.Mode().IsRegular() || current.Size() > runtimeManifestMaxBytes ||
		!os.SameFile(snapshot.info, current) || snapshot.info.Size() != current.Size() ||
		!snapshot.info.ModTime().Equal(current.ModTime()) {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(file, runtimeManifestMaxBytes+1))
	return err == nil && len(data) <= runtimeManifestMaxBytes && sha256.Sum256(data) == snapshot.digest
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func resolveExecutableInEnvironment(value string, env []string, workDir string) (string, bool) {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", false
	}
	workDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", false
	}
	if hasExecutableSeparator(value) {
		path := value
		if !filepath.IsAbs(path) {
			path = filepath.Join(workDir, path)
		}
		return executableFile(path)
	}
	pathValue := environmentValue(env, "PATH")
	if pathValue == "" {
		return "", false
	}
	for _, directory := range filepath.SplitList(pathValue) {
		if directory == "" {
			directory = workDir
		} else if !filepath.IsAbs(directory) {
			directory = filepath.Join(workDir, directory)
		}
		candidate := filepath.Join(directory, value)
		candidates := executableCandidates(candidate, env)
		for _, candidate := range candidates {
			if path, ok := executableFile(candidate); ok {
				return path, true
			}
		}
	}
	return "", false
}

func executableCandidates(path string, env []string) []string {
	if filepath.Ext(path) != "" {
		return []string{path}
	}
	if runtime.GOOS != windowsGOOS {
		return []string{path}
	}
	pathExt := environmentValue(env, "PATHEXT")
	if pathExt == "" {
		pathExt = ".COM;.EXE;.CMD"
	}
	candidates := make([]string, 0, 1+len(strings.Split(pathExt, ";")))
	candidates = append(candidates, path)
	for _, extension := range strings.Split(pathExt, ";") {
		extension = strings.TrimSpace(extension)
		if extension != "" {
			candidates = append(candidates, path+extension)
		}
	}
	return candidates
}

func executableFile(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	if runtime.GOOS != windowsGOOS && info.Mode().Perm()&0o111 == 0 {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return abs, true
}

func hasExecutableSeparator(value string) bool {
	return filepath.IsAbs(value) || strings.ContainsAny(value, "/\\")
}

func environmentKeyEqual(left, right string) bool {
	if runtime.GOOS == windowsGOOS {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func runRuntimeObservationCommand(
	ctx context.Context,
	executable string,
	args []string,
	env []string,
	workDir string,
	logger *zap.Logger,
) ([]byte, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = workDir
	cmd.Env = env
	cmd.WaitDelay = acpCommandTerminateGrace + acpCommandForceKillGrace
	var stdout, stderr boundedObservationBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	configureACPCommand(cmd, logger)
	if err := configureRuntimeObservationCommand(cmd, executable, args, env); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	lifecycle, lifecycleErr := installACPCommandLifecycle(cmd)
	if lifecycleErr != nil {
		acpCommandLogger(logger).Warn("runtime observation process lifecycle unavailable",
			zap.String("failure_class", "lifecycle_install"))
	}
	pid := cmd.Process.Pid
	waitErr := cmd.Wait()
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*acpCommandTerminateGrace+2*acpCommandForceKillGrace)
	reapACPProcessGroup(cleanupCtx, pid, logger)
	cancel()
	releaseACPCommandLifecycle(lifecycle)
	if waitErr != nil || ctx.Err() != nil || stdout.overflow || stderr.overflow {
		return nil, errors.New("runtime observation command failed")
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}
