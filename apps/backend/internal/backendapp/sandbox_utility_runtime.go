package backendapp

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/runtime"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/agent/sandboxruntime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"slices"
)

func (f sandboxRuntimeFactory) prepareUtility(ctx context.Context, probe *hostutility.ProfileProbeContext, cfg *utility.InferenceConfigDTO, transport *runtime.AgentCtlClient) (hostutility.SandboxAttempt, error) {
	if f.runtime == nil || cfg == nil || transport == nil {
		return nil, errSandboxRuntimeUnavailable
	}
	selection, err := utilitySandboxSelection(probe)
	if err != nil {
		return nil, err
	}

	if err := f.validateExecutor(ctx, selection.ExecutorID); err != nil {
		return nil, err
	}
	protected, err := f.protectedPorts(transport)
	if err != nil {
		return nil, err
	}
	prepared, err := f.runtime.Prepare(ctx, sandboxruntime.PrepareRequest{
		Selection: selection, ExecutionID: uuid.NewString(), Generation: 1,
		WorkDir: cfg.WorkDir, Arguments: append(slices.Clone(cfg.Command), cfg.CLIFlags...), ProtectedPorts: protected,
	})
	if err != nil || prepared == nil {
		return nil, err
	}
	if prepared.SelectionDigest() != probe.SandboxDigest {
		return nil, errors.Join(policy.ErrConflict, prepared.Close())
	}
	return prepared, nil
}

func utilitySandboxSelection(probe *hostutility.ProfileProbeContext) (policy.SelectionContext, error) {
	if probe == nil || probe.SandboxSelection == nil {
		return policy.SelectionContext{}, policy.ErrIneligible
	}
	selection := *probe.SandboxSelection
	if probe.SandboxDigest == "" || selection.ExecutorID == "" || probe.ExecutorID != selection.ExecutorID || probe.ProfileID != selection.AgentProfileID || selection.TaskID != "" || selection.ParentTaskID != "" || selection.WorkflowStepID != "" {
		return policy.SelectionContext{}, policy.ErrIneligible
	}
	return selection, nil
}
