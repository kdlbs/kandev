package controller

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

var (
	ErrRuntimeUpdateUnsupported   = errors.New("agent runtime update unsupported")
	ErrRuntimeUpdaterUnavailable  = errors.New("agent runtime updater unavailable")
	ErrRuntimeUpdatePreviewFailed = errors.New("agent runtime update preview failed")
)

// PreviewAgentUpdate resolves the trusted built-in update recipe without
// claiming maintenance state, creating a job, or running the command.
func (c *Controller) PreviewAgentUpdate(
	ctx context.Context,
	name string,
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
	target, err := c.runtimeUpdater.ResolveTarget(ctx, spec.Package)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRuntimeUpdatePreviewFailed, err)
	}
	command := spec.CacheUpdateCommand().Args()
	return &dto.AgentUpdatePreviewDTO{
		AgentName:      name,
		Package:        spec.Package,
		CurrentVersion: current,
		TargetVersion:  target,
		Command:        command,
		CommandString:  buildCommandString(command),
	}, nil
}

// RuntimeUpdater is the external-process and host-probe boundary used by
// managed runtime jobs. Implementations must execute commands as direct argv.
type RuntimeUpdater interface {
	CurrentCapabilities(agentName string) (hostutility.AgentCapabilities, bool)
	ResolveTarget(ctx context.Context, packageName string) (string, error)
	RunUpdate(ctx context.Context, command agents.Command, onChunk func(string)) error
	Refresh(
		ctx context.Context,
		agentName string,
		command agents.Command,
	) (hostutility.AgentCapabilities, error)
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
	return strings.TrimSpace(target), nil
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

func (u *hostRuntimeUpdater) Refresh(
	ctx context.Context,
	agentName string,
	command agents.Command,
) (hostutility.AgentCapabilities, error) {
	return u.host.RefreshWithCommand(ctx, agentName, command)
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
func (c *Controller) EnqueueAgentUpdate(name string) (*dto.AgentUpdateJobDTO, error) {
	if c.updateJobStore == nil || c.runtimeUpdater == nil {
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
	job, err := c.updateJobStore.Enqueue(name, spec)
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
