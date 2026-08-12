package controller

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

var (
	ErrRuntimeUpdateUnsupported    = errors.New("agent runtime update unsupported")
	ErrRuntimeUpdaterUnavailable   = errors.New("agent runtime updater unavailable")
	ErrRuntimeUpdatePreviewFailed  = errors.New("agent runtime update preview failed")
	ErrRuntimeUpdateTargetRequired = errors.New("managed runtime target version is required")
	ErrRuntimeUpdateTargetInvalid  = errors.New("managed runtime target version is invalid")
	ErrRuntimeUpdateTargetMissing  = errors.New("managed runtime target version is not published")
)

// PreviewAgentUpdate resolves the trusted built-in update recipe without
// claiming maintenance state, creating a job, or running the command.
func (c *Controller) PreviewAgentUpdate(
	ctx context.Context,
	name string,
	targetVersions ...string,
) (*dto.AgentUpdatePreviewDTO, error) {
	if c.runtimeUpdater == nil {
		return nil, ErrRuntimeUpdaterUnavailable
	}
	ag, ok := c.agentRegistry.Get(name)
	if !ok {
		return nil, ErrAgentNotFound
	}
	managed, ok := ag.(agents.ManagedNPMRuntimeAgent)
	if !ok {
		return nil, ErrRuntimeUpdateUnsupported
	}
	spec := managed.ManagedNPMRuntime()
	if strings.TrimSpace(spec.Package) == "" {
		return nil, ErrRuntimeUpdateUnsupported
	}

	current := ""
	if caps, found := c.runtimeUpdater.CurrentCapabilities(name); found {
		current = caps.AgentVersion
	}
	active, err := c.activeRuntimeVersion(ctx, name, spec.Package)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRuntimeUpdatePreviewFailed, err)
	}
	catalogue, exactCatalogue, err := c.resolveRuntimeCatalogue(ctx, spec.Package, active, current)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRuntimeUpdatePreviewFailed, err)
	}
	target := catalogue.Latest
	if len(targetVersions) > 0 && strings.TrimSpace(targetVersions[0]) != "" {
		target = strings.TrimSpace(targetVersions[0])
		if _, err := managedruntime.ParseStableVersion(target); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrRuntimeUpdateTargetInvalid, err)
		}
		if exactCatalogue && !catalogue.Has(target) {
			return nil, fmt.Errorf("%w: %s", ErrRuntimeUpdateTargetMissing, target)
		}
	}
	operation, err := managedruntime.ClassifyOperation(active, current, target)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRuntimeUpdateTargetInvalid, err)
	}
	command := spec.CacheUpdateCommand(target).Args()
	if !exactCatalogue && (len(targetVersions) == 0 || strings.TrimSpace(targetVersions[0]) == "") {
		// Keep the compatibility preview for embedders that provide only the
		// legacy latest-version seam. Production uses the catalogue resolver and
		// always previews an exact package@version command.
		command = spec.CacheUpdateCommand().Args()
	}
	return &dto.AgentUpdatePreviewDTO{
		AgentName:         name,
		Package:           spec.Package,
		CurrentVersion:    current,
		ActiveVersion:     active,
		TargetVersion:     target,
		Operation:         string(operation),
		AvailableVersions: runtimeVersionDTOs(catalogue),
		Command:           command,
		CommandString:     buildCommandString(command),
	}, nil
}

func (c *Controller) resolveRuntimeCatalogue(
	ctx context.Context,
	packageName string,
	extras ...string,
) (managedruntime.Catalogue, bool, error) {
	if resolver, ok := c.runtimeUpdater.(RuntimeVersionResolver); ok {
		metadata, err := resolver.ResolveVersions(ctx, packageName)
		if err != nil {
			return managedruntime.Catalogue{}, true, err
		}
		catalogue, err := managedruntime.BuildCatalogue(metadata.Versions, metadata.Latest, extras...)
		return catalogue, true, err
	}
	target, err := c.runtimeUpdater.ResolveTarget(ctx, packageName)
	if err != nil {
		return managedruntime.Catalogue{}, false, err
	}
	catalogue, err := managedruntime.BuildCatalogue([]string{target}, target)
	return catalogue, false, err
}

func runtimeVersionDTOs(catalogue managedruntime.Catalogue) []dto.AgentUpdateVersionDTO {
	versions := make([]dto.AgentUpdateVersionDTO, 0, len(catalogue.Versions))
	for _, version := range catalogue.Versions {
		versions = append(versions, dto.AgentUpdateVersionDTO{
			Version: version.Version,
			Latest:  version.Latest,
		})
	}
	return versions
}

func (c *Controller) activeRuntimeVersion(
	ctx context.Context,
	agentName string,
	packageName string,
) (string, error) {
	if c.managedRuntimeSelections == nil {
		return "", nil
	}
	selection, found, err := c.managedRuntimeSelections.Get(ctx, agentName, packageName)
	if err != nil {
		return "", fmt.Errorf("read active runtime version: %w", err)
	}
	if !found {
		return "", nil
	}
	return selection.Version, nil
}

// RuntimeUpdater is the external-process and host-probe boundary used by
// managed runtime jobs. Implementations must execute commands as direct argv.
type RuntimeUpdater interface {
	CurrentCapabilities(agentName string) (hostutility.AgentCapabilities, bool)
	ResolveTarget(ctx context.Context, packageName string) (string, error)
	RunUpdate(ctx context.Context, command agents.Command, onChunk func(string)) error
	InvalidateExecutionCache(ctx context.Context, packageName string) error
	Refresh(
		ctx context.Context,
		agentName string,
		command agents.Command,
	) (hostutility.AgentCapabilities, error)
}

// RuntimeVersionMetadata is the trusted npm catalogue used to authorize an
// exact target after the browser preview has been shown.
type RuntimeVersionMetadata struct {
	Versions []string
	Latest   string
}

// RuntimeVersionResolver obtains package metadata without executing a package.
type RuntimeVersionResolver interface {
	ResolveVersions(context.Context, string) (RuntimeVersionMetadata, error)
}

// RuntimeCandidateUpdater separates validation probes from live capability
// publication. The update job persists the selection between these calls.
type RuntimeCandidateUpdater interface {
	Probe(context.Context, string, agents.Command) (hostutility.AgentCapabilities, error)
	PublishCapabilities(string, hostutility.AgentCapabilities)
}

// ExactRuntimeCacheInvalidator removes only the version-specific npm tree.
type ExactRuntimeCacheInvalidator interface {
	InvalidateExecutionCacheVersion(context.Context, string, string) error
}

type hostRuntimeUpdater struct {
	host     *hostutility.Manager
	executor directCommandExecutor
}

type directCommandExecutor interface {
	Output(ctx context.Context, command agents.Command) (string, error)
	Stream(ctx context.Context, command agents.Command, onChunk func(string)) error
}

type execDirectCommandExecutor struct{}

func (execDirectCommandExecutor) Output(
	ctx context.Context,
	command agents.Command,
) (string, error) {
	return runDirectCommandOutput(ctx, command)
}

func (execDirectCommandExecutor) Stream(
	ctx context.Context,
	command agents.Command,
	onChunk func(string),
) error {
	return runDirectCommand(ctx, command, onChunk)
}

func (u *hostRuntimeUpdater) CurrentCapabilities(agentName string) (hostutility.AgentCapabilities, bool) {
	return u.host.Get(agentName)
}

func (u *hostRuntimeUpdater) ResolveTarget(ctx context.Context, packageName string) (string, error) {
	command := agents.NewCommand("npm", "view", packageName, "dist-tags.latest", "--json")
	output, err := u.executor.Output(ctx, command)
	if err != nil {
		return "", err
	}
	var target string
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &target); err != nil {
		return "", fmt.Errorf("parse npm target version: %w", err)
	}
	if strings.TrimSpace(target) == "" {
		return "", errors.New("npm target version is empty")
	}
	target = strings.TrimSpace(target)
	if _, err := managedruntime.ParseStableVersion(target); err != nil {
		return "", fmt.Errorf("npm target version is not stable: %w", err)
	}
	return target, nil
}

func (u *hostRuntimeUpdater) ResolveVersions(
	ctx context.Context,
	packageName string,
) (RuntimeVersionMetadata, error) {
	command := agents.NewCommand("npm", "view", packageName, "versions", "dist-tags", "--json")
	output, err := u.executor.Output(ctx, command)
	if err != nil {
		return RuntimeVersionMetadata{}, err
	}
	var metadata struct {
		Versions []string          `json:"versions"`
		DistTags map[string]string `json:"dist-tags"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &metadata); err != nil {
		return RuntimeVersionMetadata{}, fmt.Errorf("parse npm runtime metadata: %w", err)
	}
	latest := ""
	if metadata.DistTags != nil {
		latest = strings.TrimSpace(metadata.DistTags["latest"])
	}
	if latest == "" {
		return RuntimeVersionMetadata{}, errors.New("npm latest runtime version is empty")
	}
	return RuntimeVersionMetadata{Versions: metadata.Versions, Latest: latest}, nil
}

func runDirectCommandOutput(ctx context.Context, command agents.Command) (string, error) {
	argv := command.Args()
	if len(argv) == 0 {
		return "", errors.New("runtime update command is empty")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = filteredInstallEnv()
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (u *hostRuntimeUpdater) RunUpdate(
	ctx context.Context,
	command agents.Command,
	onChunk func(string),
) error {
	return u.executor.Stream(ctx, command, onChunk)
}

func (u *hostRuntimeUpdater) npxCacheRoot(ctx context.Context) (string, error) {
	output, err := u.executor.Output(ctx, agents.NewCommand("npm", "config", "get", "cache"))
	if err != nil {
		return "", fmt.Errorf("resolve npm cache root: %w", err)
	}
	cacheRoot := strings.TrimSpace(output)
	if !filepath.IsAbs(cacheRoot) {
		return "", fmt.Errorf("npm cache root is not absolute: %q", cacheRoot)
	}
	cacheRoot = filepath.Clean(cacheRoot)
	if cacheRoot == string(filepath.Separator) {
		return "", errors.New("refusing to invalidate execution cache under filesystem root")
	}
	npxRoot := filepath.Join(cacheRoot, "_npx")
	if info, statErr := os.Lstat(npxRoot); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refusing to invalidate execution cache through symlink: %s", npxRoot)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", fmt.Errorf("inspect npm execution cache: %w", statErr)
	}
	return npxRoot, nil
}

func removeNpmExecutionCacheKey(npxRoot, key string) error {
	target := filepath.Join(npxRoot, key)
	rel, err := filepath.Rel(npxRoot, target)
	if err != nil || rel != key {
		return fmt.Errorf("invalid npm execution cache target: %s", target)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("remove npm execution cache %s: %w", target, err)
	}
	return nil
}

func (u *hostRuntimeUpdater) InvalidateExecutionCache(ctx context.Context, packageName string) error {
	packageName = strings.TrimSpace(packageName)
	if packageName == "" {
		return errors.New("managed runtime package is empty")
	}
	npxRoot, err := u.npxCacheRoot(ctx)
	if err != nil {
		return err
	}
	key := (agents.ManagedNPMRuntimeSpec{Package: packageName}).ExecutionCacheKey()
	return removeNpmExecutionCacheKey(npxRoot, key)
}

func (u *hostRuntimeUpdater) InvalidateExecutionCacheVersion(
	ctx context.Context,
	packageName string,
	version string,
) error {
	packageName = strings.TrimSpace(packageName)
	version = strings.TrimSpace(version)
	if packageName == "" || version == "" {
		return errors.New("managed runtime package and version are required")
	}
	if _, err := managedruntime.ParseStableVersion(version); err != nil {
		return err
	}
	npxRoot, err := u.npxCacheRoot(ctx)
	if err != nil {
		return err
	}
	spec := agents.ManagedNPMRuntimeSpec{Package: packageName}
	return removeNpmExecutionCacheKey(npxRoot, spec.ExecutionCacheKey(version))
}

func (u *hostRuntimeUpdater) Refresh(
	ctx context.Context,
	agentName string,
	command agents.Command,
) (hostutility.AgentCapabilities, error) {
	return u.host.RefreshWithCommand(ctx, agentName, command)
}

func (u *hostRuntimeUpdater) Probe(
	ctx context.Context,
	agentName string,
	command agents.Command,
) (hostutility.AgentCapabilities, error) {
	return u.host.ProbeWithCommand(ctx, agentName, command)
}

func (u *hostRuntimeUpdater) PublishCapabilities(
	agentName string,
	caps hostutility.AgentCapabilities,
) {
	u.host.PublishCapabilities(agentName, caps)
}

func runDirectCommand(ctx context.Context, command agents.Command, onChunk func(string)) error {
	argv := command.Args()
	if len(argv) == 0 {
		return errors.New("runtime update command is empty")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = filteredInstallEnv()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go scanCommandOutput(stdout, onChunk, &wg, errCh)
	go scanCommandOutput(stderr, onChunk, &wg, errCh)
	wg.Wait()
	close(errCh)
	if err := cmd.Wait(); err != nil {
		return err
	}
	for scanErr := range errCh {
		if scanErr != nil {
			return scanErr
		}
	}
	return nil
}

func scanCommandOutput(
	reader io.Reader,
	onChunk func(string),
	wg *sync.WaitGroup,
	errCh chan<- error,
) {
	defer wg.Done()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		onChunk(scanner.Text() + "\n")
	}
	if err := scanner.Err(); err != nil {
		errCh <- err
	}
}

// EnqueueAgentUpdate starts or reuses an update for a built-in managed agent.
func (c *Controller) EnqueueAgentUpdate(
	ctx context.Context,
	name string,
	targetVersion string,
) (*dto.AgentUpdateJobDTO, error) {
	if c.updateJobStore == nil || c.runtimeUpdater == nil {
		return nil, ErrRuntimeUpdaterUnavailable
	}
	if active, found := c.updateJobStore.GetActive(name); found {
		return active, nil
	}
	ag, ok := c.agentRegistry.Get(name)
	if !ok {
		return nil, ErrAgentNotFound
	}
	managed, ok := ag.(agents.ManagedNPMRuntimeAgent)
	if !ok {
		return nil, ErrRuntimeUpdateUnsupported
	}
	spec := managed.ManagedNPMRuntime()
	if strings.TrimSpace(spec.Package) == "" {
		return nil, ErrRuntimeUpdateUnsupported
	}
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" {
		return nil, ErrRuntimeUpdateTargetRequired
	}
	if _, err := managedruntime.ParseStableVersion(targetVersion); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRuntimeUpdateTargetInvalid, err)
	}
	if resolver, ok := c.runtimeUpdater.(RuntimeVersionResolver); ok {
		metadata, err := resolver.ResolveVersions(ctx, spec.Package)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrRuntimeUpdatePreviewFailed, err)
		}
		catalogue, err := managedruntime.BuildCatalogue(metadata.Versions, metadata.Latest)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrRuntimeUpdatePreviewFailed, err)
		}
		if !catalogue.Has(targetVersion) {
			return nil, fmt.Errorf("%w: %s", ErrRuntimeUpdateTargetMissing, targetVersion)
		}
	}
	job, err := c.updateJobStore.Enqueue(name, spec, targetVersion)
	if err != nil {
		return nil, err
	}
	if snapshot, found := c.updateJobStore.Get(job.ID); found {
		return snapshot, nil
	}
	snapshot := job.snapshot()
	return &snapshot, nil
}

func (c *Controller) ListAgentUpdateJobs() []dto.AgentUpdateJobDTO {
	if c.updateJobStore == nil {
		return nil
	}
	return c.updateJobStore.ListAll()
}

func (c *Controller) GetAgentUpdateJob(id string) (*dto.AgentUpdateJobDTO, bool) {
	if c.updateJobStore == nil {
		return nil, false
	}
	return c.updateJobStore.Get(id)
}
