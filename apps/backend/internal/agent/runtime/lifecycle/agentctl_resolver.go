package lifecycle

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

const (
	remoteHelperDownloadTimeout  = 90 * time.Second
	remoteHelperInventoryTimeout = 2 * time.Second
	remoteHelperPruneCooldown    = 15 * time.Second
	remoteHelperCacheDir         = "remote-helpers"
	defaultReleaseBaseURL        = "https://github.com/kdlbs/kandev/releases/download"
)

// AgentctlResolverOptions bind resolution to the running release and its
// writable Kandev home. ReleaseBaseURL and HTTPClient are injectable for tests.
type AgentctlResolverOptions struct {
	Version         string
	Commit          string
	BundleDir       string
	HomeDir         string
	ReleaseBaseURL  string
	HTTPClient      *http.Client
	DownloadTimeout time.Duration
}

// AgentctlResolver finds verified platform-specific agentctl helpers for
// remote executors.
type AgentctlResolver struct {
	logger  *logger.Logger
	options AgentctlResolverOptions

	downloadMu sync.Mutex
	downloads  map[string]*remoteHelperDownload

	cacheInventoryMu  sync.RWMutex
	cacheInventory    RemoteHelperCacheMountInventory
	cachePruneMu      sync.Mutex
	lastCachePrune    time.Time
	cachePruneRunning bool
	cachePruneDone    chan struct{}
}

type remoteHelperDownload struct {
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	waiters   int
	canceled  bool
	completed bool
	err       error
}

// RemoteHelperCacheMountInventory returns every host path mounted by a managed
// container. An error means the inventory is incomplete and pruning must defer.
type RemoteHelperCacheMountInventory func(context.Context) ([]string, error)

// NewAgentctlResolver creates a legacy-compatible resolver. Packaged backend
// construction should use NewAgentctlResolverWithOptions with its build identity.
func NewAgentctlResolver(log *logger.Logger) *AgentctlResolver {
	return NewAgentctlResolverWithOptions(log, AgentctlResolverOptions{})
}

// NewAgentctlResolverWithOptions creates a resolver configured for a release
// bundle. Empty options retain source-build and legacy bundle behavior.
func NewAgentctlResolverWithOptions(log *logger.Logger, options AgentctlResolverOptions) *AgentctlResolver {
	if options.ReleaseBaseURL == "" {
		options.ReleaseBaseURL = defaultReleaseBaseURL
	}
	if options.HTTPClient == nil {
		options.HTTPClient = http.DefaultClient
	}
	if options.DownloadTimeout <= 0 || options.DownloadTimeout > remoteHelperDownloadTimeout {
		options.DownloadTimeout = remoteHelperDownloadTimeout
	}
	return &AgentctlResolver{logger: log.WithFields(zap.String("component", "agentctl_resolver")), options: options}
}

// SetCacheMountInventory wires the complete managed-container mount inventory
// used by cache pruning across every remote executor.
func (r *AgentctlResolver) SetCacheMountInventory(inventory RemoteHelperCacheMountInventory) {
	r.cacheInventoryMu.Lock()
	defer r.cacheInventoryMu.Unlock()
	r.cacheInventory = inventory
}

// ResolveLinuxBinary returns the path to a linux/amd64 agentctl binary.
func (r *AgentctlResolver) ResolveLinuxBinary() (string, error) {
	return r.ResolveLinuxBinaryContext(context.Background(), nil)
}

// ResolveLinuxBinaryContext resolves linux/amd64 with launch cancellation and
// progress reporting.
func (r *AgentctlResolver) ResolveLinuxBinaryContext(ctx context.Context, onProgress PrepareProgressCallback) (string, error) {
	return r.ResolveRemoteBinaryContext(ctx, SSHRemotePlatform{GOOS: sshRemoteGOOSLinux, GOARCH: sshRemoteGOARCHAMD64}, onProgress)
}

// ResolveRemoteBinary retains compatibility for callers that do not have a
// launch context. New executor paths should use ResolveRemoteBinaryContext.
func (r *AgentctlResolver) ResolveRemoteBinary(platform SSHRemotePlatform) (string, error) {
	return r.ResolveRemoteBinaryContext(context.Background(), platform, nil)
}

// ResolveRemoteBinaryContext resolves an exact remote helper. Explicit path
// overrides retain precedence; manifest bundles never fall back to a native
// host binary.
func (r *AgentctlResolver) ResolveRemoteBinaryContext(ctx context.Context, platform SSHRemotePlatform, onProgress PrepareProgressCallback) (string, error) {
	if err := requireSupportedRemotePlatform(platform); err != nil {
		return "", err
	}
	for _, envName := range agentctlBinaryEnvNames(platform) {
		if envPath := os.Getenv(envName); envPath != "" {
			info, err := os.Stat(envPath)
			if err == nil && info.Mode().IsRegular() {
				r.logger.Debug("using agentctl from env var", zap.String("env", envName), zap.String("path", envPath), zap.String("remote_platform", platform.String()))
				return envPath, nil
			}
			return "", fmt.Errorf("%s=%q does not exist or is not a file", envName, envPath)
		}
	}

	root := r.bundleRoot()
	if root != "" {
		manifest, hasManifest, err := ReadRemoteHelperManifest(root, r.options.Version, r.options.Commit)
		if err != nil {
			return "", err
		}
		if hasManifest {
			return r.resolveManifestHelper(ctx, root, manifest, platform, onProgress)
		}
	}
	if candidate := r.findLegacyHelper(root, platform); candidate != "" {
		return candidate, nil
	}
	return "", fmt.Errorf("agentctl helper for %s not found; build it with 'make build-agentctl-remote' or set %s (control plane os=%s arch=%s)",
		platform.String(), agentctlBinaryEnvNames(platform)[0], runtime.GOOS, runtime.GOARCH)
}

func (r *AgentctlResolver) bundleRoot() string {
	if r.options.BundleDir != "" {
		return r.options.BundleDir
	}
	if dir := os.Getenv("KANDEV_BUNDLE_DIR"); dir != "" {
		return dir
	}
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	binDir := filepath.Dir(exePath)
	if filepath.Base(binDir) != "bin" {
		return ""
	}
	return filepath.Dir(binDir)
}

func (r *AgentctlResolver) expectedRemoteHelperSHA256(platform SSHRemotePlatform) (string, bool, error) {
	if err := requireSupportedRemotePlatform(platform); err != nil {
		return "", false, err
	}
	for _, envName := range agentctlBinaryEnvNames(platform) {
		if envPath := os.Getenv(envName); envPath != "" {
			data, err := os.ReadFile(envPath)
			if err != nil {
				return "", false, fmt.Errorf("read %s=%q: %w", envName, envPath, err)
			}
			digest := sha256.Sum256(data)
			return hex.EncodeToString(digest[:]), true, nil
		}
	}
	root := r.bundleRoot()
	if root == "" {
		return "", false, nil
	}
	manifest, hasManifest, err := ReadRemoteHelperManifest(root, r.options.Version, r.options.Commit)
	if err != nil || !hasManifest {
		return "", hasManifest, err
	}
	record, ok := remoteHelperForPlatform(manifest, platform)
	if !ok {
		return "", true, fmt.Errorf("remote helper manifest has no record for %s", platform.String())
	}
	if manifest.Variant == RemoteHelperVariantFull {
		path := filepath.Join(root, "bin", agentctlBinaryName(platform))
		if err := validateBundledRemoteHelper(path, record); err != nil {
			return "", true, err
		}
	}
	return record.SHA256, true, nil
}

func (r *AgentctlResolver) findLegacyHelper(root string, platform SSHRemotePlatform) string {
	candidates := make([]string, 0, 8)
	if root != "" {
		candidates = append(candidates, filepath.Join(root, "bin", agentctlBinaryName(platform)))
	}
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates, agentctlBinaryCandidates(filepath.Dir(exePath), platform)...)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			abs, _ := filepath.Abs(candidate)
			r.logger.Debug("found legacy agentctl helper", zap.String("path", abs), zap.String("remote_platform", platform.String()))
			return abs
		}
	}
	return ""
}

func (r *AgentctlResolver) resolveManifestHelper(ctx context.Context, root string, manifest *RemoteHelperManifest, platform SSHRemotePlatform, onProgress PrepareProgressCallback) (string, error) {
	record, ok := remoteHelperForPlatform(manifest, platform)
	if !ok {
		return "", fmt.Errorf("remote helper manifest has no record for %s", platform.String())
	}
	switch manifest.Variant {
	case RemoteHelperVariantFull:
		return resolveBundledManifestHelper(root, platform, record)
	case RemoteHelperVariantStandard:
		return r.resolveStandardManifestHelper(ctx, manifest, record, platform, onProgress)
	default:
		return "", fmt.Errorf("remote helper manifest variant %q is unsupported", manifest.Variant)
	}
}

func resolveBundledManifestHelper(root string, platform SSHRemotePlatform, record RemoteHelperRecord) (string, error) {
	path := filepath.Join(root, "bin", agentctlBinaryName(platform))
	if err := validateBundledRemoteHelper(path, record); err != nil {
		return "", err
	}
	return path, nil
}

func (r *AgentctlResolver) resolveStandardManifestHelper(ctx context.Context, manifest *RemoteHelperManifest, record RemoteHelperRecord, platform SSHRemotePlatform, onProgress PrepareProgressCallback) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cachePath, err := r.cachePath(manifest, record)
	if err != nil {
		return "", err
	}
	cacheRoot := filepath.Join(r.options.HomeDir, "cache", remoteHelperCacheDir)
	lease, err := pinRemoteHelperCachePath(cacheRoot, cachePath, ctx)
	if err != nil {
		return "", err
	}
	keepLease := false
	defer func() {
		if !keepLease {
			if err := lease.Release(); err != nil {
				r.logger.Warn("failed to release remote helper cache lease", zap.Error(err))
			}
		}
	}()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if valid, err := validateCachedRemoteHelper(cachePath, record); err != nil {
		return "", err
	} else if valid {
		r.logger.Debug("using verified remote helper cache", zap.String("path", cachePath), zap.String("remote_platform", platform.String()))
		r.scheduleCachePruneAfterResolution()
		keepLease = r.retainRemoteHelperCacheLease(ctx, lease)
		if !keepLease {
			return "", ctx.Err()
		}
		return cachePath, nil
	}
	started := time.Now().UTC()
	emitRemoteHelperProgress(onProgress, platform, PrepareStepRunning, started, nil)
	if err := r.awaitRemoteHelperDownload(ctx, manifest, record, cachePath); err != nil {
		err = fmt.Errorf("resolve remote agentctl for %s: %w; use the full offline runtime archive or set %s", platform.String(), err, agentctlBinaryEnvNames(platform)[0])
		emitRemoteHelperProgress(onProgress, platform, PrepareStepFailed, started, err)
		return "", err
	}
	if err := validateDownloadedRemoteHelper(cachePath, record); err != nil {
		emitRemoteHelperProgress(onProgress, platform, PrepareStepFailed, started, err)
		return "", err
	}
	emitRemoteHelperProgress(onProgress, platform, PrepareStepCompleted, started, nil)
	r.scheduleCachePruneAfterResolution()
	keepLease = r.retainRemoteHelperCacheLease(ctx, lease)
	if !keepLease {
		return "", ctx.Err()
	}
	return cachePath, nil
}

func (r *AgentctlResolver) awaitRemoteHelperDownload(ctx context.Context, manifest *RemoteHelperManifest, record RemoteHelperRecord, cachePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	flight, start := r.joinRemoteHelperDownload(cachePath)
	if start {
		go r.runRemoteHelperDownload(cachePath, flight, manifest, record)
	}
	select {
	case <-ctx.Done():
		r.leaveRemoteHelperDownload(cachePath, flight)
		return ctx.Err()
	case <-flight.done:
		r.leaveRemoteHelperDownload(cachePath, flight)
		if err := ctx.Err(); err != nil {
			return err
		}
		return flight.err
	}
}

func (r *AgentctlResolver) joinRemoteHelperDownload(key string) (*remoteHelperDownload, bool) {
	r.downloadMu.Lock()
	defer r.downloadMu.Unlock()
	if r.downloads == nil {
		r.downloads = make(map[string]*remoteHelperDownload)
	}
	flight := r.downloads[key]
	if flight != nil && flight.canceled {
		delete(r.downloads, key)
		flight = nil
	}
	start := flight == nil
	if start {
		transferCtx, cancel := context.WithTimeout(context.Background(), r.downloadTimeout())
		flight = &remoteHelperDownload{ctx: transferCtx, cancel: cancel, done: make(chan struct{})}
		r.downloads[key] = flight
	}
	flight.waiters++
	return flight, start
}

func (r *AgentctlResolver) runRemoteHelperDownload(key string, flight *remoteHelperDownload, manifest *RemoteHelperManifest, record RemoteHelperRecord) {
	err := r.downloadRemoteHelper(flight.ctx, manifest, record, key)
	flight.cancel()
	r.downloadMu.Lock()
	flight.err = err
	flight.completed = true
	if r.downloads[key] == flight {
		delete(r.downloads, key)
	}
	close(flight.done)
	r.downloadMu.Unlock()
}

func (r *AgentctlResolver) leaveRemoteHelperDownload(key string, flight *remoteHelperDownload) {
	r.downloadMu.Lock()
	if flight.waiters > 0 {
		flight.waiters--
	}
	if flight.waiters == 0 && !flight.completed {
		flight.canceled = true
		if r.downloads[key] == flight {
			delete(r.downloads, key)
		}
		flight.cancel()
	}
	r.downloadMu.Unlock()
}

func (r *AgentctlResolver) downloadTimeout() time.Duration {
	if r.options.DownloadTimeout <= 0 || r.options.DownloadTimeout > remoteHelperDownloadTimeout {
		return remoteHelperDownloadTimeout
	}
	return r.options.DownloadTimeout
}

func (r *AgentctlResolver) retainRemoteHelperCacheLease(ctx context.Context, lease *remoteHelperCacheLease) bool {
	if err := ctx.Err(); err != nil {
		return false
	}
	context.AfterFunc(ctx, func() {
		if err := lease.Release(); err != nil {
			r.logger.Warn("failed to release remote helper cache lease", zap.Error(err))
		}
	})
	return ctx.Err() == nil
}

func validateDownloadedRemoteHelper(cachePath string, record RemoteHelperRecord) error {
	valid, err := validateCachedRemoteHelper(cachePath, record)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("downloaded helper failed cache validation")
	}
	return nil
}

func (r *AgentctlResolver) cachePath(manifest *RemoteHelperManifest, record RemoteHelperRecord) (string, error) {
	if r.options.HomeDir == "" {
		return "", errors.New("kandev home directory is not configured for the remote helper cache")
	}
	if filepath.IsAbs(manifest.Version) || strings.ContainsAny(manifest.Version, `/\\`) || manifest.Version == "." || manifest.Version == ".." {
		return "", errors.New("remote helper release version cannot be used as a cache path")
	}
	platform := strings.ReplaceAll(record.Platform, "/", "-")
	return filepath.Join(r.options.HomeDir, "cache", remoteHelperCacheDir, manifest.Version, platform, record.SHA256, "agentctl"), nil
}

func (r *AgentctlResolver) pruneCache(pinnedPaths []string, inventoryComplete bool) error {
	if r.options.HomeDir == "" || r.options.Version == "" {
		return nil
	}
	_, err := PruneRemoteHelperCache(filepath.Join(r.options.HomeDir, "cache", remoteHelperCacheDir), r.options.Version, pinnedPaths, inventoryComplete)
	return err
}

func (r *AgentctlResolver) scheduleCachePruneAfterResolution() {
	r.cacheInventoryMu.RLock()
	inventory := r.cacheInventory
	r.cacheInventoryMu.RUnlock()
	if inventory == nil {
		return
	}

	r.cachePruneMu.Lock()
	now := time.Now()
	if r.cachePruneRunning || now.Sub(r.lastCachePrune) < remoteHelperPruneCooldown {
		r.cachePruneMu.Unlock()
		return
	}
	r.lastCachePrune = now
	r.cachePruneRunning = true
	done := make(chan struct{})
	r.cachePruneDone = done
	r.cachePruneMu.Unlock()

	go func() {
		defer func() {
			r.cachePruneMu.Lock()
			r.cachePruneRunning = false
			close(done)
			r.cachePruneMu.Unlock()
		}()

		inventoryCtx, cancel := context.WithTimeout(context.Background(), remoteHelperInventoryTimeout)
		defer cancel()
		pinnedPaths, err := inventory(inventoryCtx)
		if err != nil {
			r.logger.Debug("defer remote helper cache cleanup because container inventory is unavailable", zap.Error(err))
			return
		}
		if err := r.pruneCache(pinnedPaths, true); err != nil {
			r.logger.Warn("failed to prune unused remote helper cache entries", zap.Error(err))
		}
	}()
}

func (r *AgentctlResolver) downloadRemoteHelper(ctx context.Context, manifest *RemoteHelperManifest, record RemoteHelperRecord, cachePath string) error {
	downloadCtx, cancel, err := withRemoteHelperTimeout(ctx, r.options.DownloadTimeout)
	if err != nil {
		return err
	}
	defer cancel()
	response, err := r.requestRemoteHelperAsset(downloadCtx, manifest, record)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	tempPath, err := stageVerifiedRemoteHelper(cachePath, record, response.Body)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tempPath) }()
	return publishVerifiedRemoteHelper(tempPath, cachePath, record)
}

func (r *AgentctlResolver) requestRemoteHelperAsset(ctx context.Context, manifest *RemoteHelperManifest, record RemoteHelperRecord) (*http.Response, error) {
	releaseVersion := strings.TrimPrefix(manifest.Version, "v")
	assetURL := strings.TrimRight(r.options.ReleaseBaseURL, "/") + "/v" + url.PathEscape(releaseVersion) + "/" + url.PathEscape(record.Asset)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create helper request: %w", err)
	}
	response, err := r.options.HTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download helper asset: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, fmt.Errorf("download helper asset returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxRemoteHelperSizeBytes {
		_ = response.Body.Close()
		return nil, fmt.Errorf("compressed helper asset exceeds %d bytes", maxRemoteHelperSizeBytes)
	}
	return response, nil
}

func stageVerifiedRemoteHelper(cachePath string, record RemoteHelperRecord, archive io.Reader) (string, error) {
	compressed := &io.LimitedReader{R: archive, N: maxRemoteHelperSizeBytes + 1}
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		return "", fmt.Errorf("open compressed helper asset: %w", err)
	}
	defer func() { _ = reader.Close() }()
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return "", fmt.Errorf("create remote helper cache: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(cachePath), ".agentctl-download-*")
	if err != nil {
		return "", fmt.Errorf("create temporary helper file: %w", err)
	}
	tempPath := temporary.Name()
	keepTemp := false
	defer func() {
		_ = temporary.Close()
		if !keepTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := writeVerifiedRemoteHelper(reader, compressed, temporary, record); err != nil {
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		return "", fmt.Errorf("sync temporary helper file: %w", err)
	}
	if err := temporary.Chmod(0o755); err != nil {
		return "", fmt.Errorf("make helper executable: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close temporary helper file: %w", err)
	}
	keepTemp = true
	return tempPath, nil
}

func writeVerifiedRemoteHelper(reader io.Reader, compressed *io.LimitedReader, temporary *os.File, record RemoteHelperRecord) error {
	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(temporary, hasher), io.LimitReader(reader, record.SizeBytes+1))
	if err != nil {
		return fmt.Errorf("read decompressed helper asset: %w", err)
	}
	if compressed.N == 0 {
		return fmt.Errorf("compressed helper asset exceeds %d bytes", maxRemoteHelperSizeBytes)
	}
	if size != record.SizeBytes {
		return fmt.Errorf("helper size mismatch: received %d bytes, expected %d", size, record.SizeBytes)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != record.SHA256 {
		return fmt.Errorf("helper sha256 mismatch: received %s", got)
	}
	return nil
}

func publishVerifiedRemoteHelper(tempPath, cachePath string, record RemoteHelperRecord) error {
	if err := os.Rename(tempPath, cachePath); err != nil {
		if valid, verifyErr := validateCachedRemoteHelper(cachePath, record); verifyErr == nil && valid {
			return nil
		}
		return fmt.Errorf("publish verified helper to cache: %w", err)
	}
	return nil
}

func validateCachedRemoteHelper(path string, record RemoteHelperRecord) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open cached helper: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("stat cached helper: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != record.SizeBytes {
		return false, nil
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, io.LimitReader(file, record.SizeBytes+1)); err != nil {
		return false, fmt.Errorf("read cached helper: %w", err)
	}
	if hex.EncodeToString(hasher.Sum(nil)) != record.SHA256 {
		return false, nil
	}
	if info.Mode().Perm()&0o111 == 0 {
		if err := os.Chmod(path, 0o755); err != nil {
			return false, fmt.Errorf("make cached helper executable: %w", err)
		}
	}
	return true, nil
}

func withRemoteHelperTimeout(ctx context.Context, maximum time.Duration) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if maximum <= 0 || maximum > remoteHelperDownloadTimeout {
		maximum = remoteHelperDownloadTimeout
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, nil, context.DeadlineExceeded
		}
		if remaining < maximum {
			maximum = remaining
		}
	}
	child, cancel := context.WithTimeout(ctx, maximum)
	return child, cancel, nil
}

func emitRemoteHelperProgress(callback PrepareProgressCallback, platform SSHRemotePlatform, status PrepareStepStatus, started time.Time, err error) {
	if callback == nil {
		return
	}
	step := PrepareStep{Kind: PrepareStepKindRemoteHelperDownload, RemotePlatform: platform.String(), Status: status, StartedAt: &started}
	if status == PrepareStepCompleted || status == PrepareStepFailed {
		ended := time.Now().UTC()
		step.EndedAt = &ended
	}
	if err != nil {
		step.Error = err.Error()
		if errors.Is(err, context.DeadlineExceeded) {
			step.FailureCode = "timeout"
		} else {
			step.FailureCode = "download_failed"
		}
	}
	callback(step, 0, 1)
}

func agentctlBinaryName(platform SSHRemotePlatform) string {
	return fmt.Sprintf("agentctl-%s-%s", platform.GOOS, platform.GOARCH)
}

func agentctlBinaryEnvNames(platform SSHRemotePlatform) []string {
	primary := fmt.Sprintf("KANDEV_AGENTCTL_%s_%s_BINARY", strings.ToUpper(platform.GOOS), strings.ToUpper(platform.GOARCH))
	if platform.GOOS == sshRemoteGOOSLinux && platform.GOARCH == sshRemoteGOARCHAMD64 {
		return []string{primary, "KANDEV_AGENTCTL_LINUX_BINARY"}
	}
	return []string{primary}
}

func agentctlBinaryCandidates(exeDir string, platform SSHRemotePlatform) []string {
	name := agentctlBinaryName(platform)
	candidates := []string{filepath.Join(exeDir, name), filepath.Join(exeDir, "..", "build", name), filepath.Join(exeDir, "..", "bin", name)}
	if platform.GOOS == runtime.GOOS && platform.GOARCH == runtime.GOARCH {
		candidates = append(candidates, filepath.Join(exeDir, "agentctl"), filepath.Join(exeDir, "..", "build", "agentctl"), filepath.Join(exeDir, "..", "bin", "agentctl"))
	}
	return candidates
}
