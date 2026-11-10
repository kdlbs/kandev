package controller

import (
	"context"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

var (
	// ErrRuntimeReviewChanged rejects admission when the trusted runtime no
	// longer matches the evidence returned by the reviewed preview.
	ErrRuntimeReviewChanged = errors.New("agent runtime review changed")
	// ErrRuntimeReviewInvalid rejects malformed or incompatible review evidence.
	ErrRuntimeReviewInvalid = errors.New("agent runtime review evidence is invalid")
)

func reviewedRuntimeID(packageName string) string {
	return "npm:" + packageName
}

func reviewedEvidence(
	packageName string,
	current, active, effective string,
	family managedruntime.OpenCodeFamily,
	source managedruntime.OpenCodeSource,
	revision uint64,
) *dto.AgentRuntimeReviewDTO {
	if current == "" || effective == "" {
		return nil
	}
	if family != "" && source != managedruntime.OpenCodeSourceManaged {
		return nil
	}
	return &dto.AgentRuntimeReviewDTO{
		RuntimeID:        reviewedRuntimeID(packageName),
		Package:          packageName,
		CurrentVersion:   current,
		EffectiveVersion: effective,
		ActiveVersion:    active,
		Family:           string(family),
		Source:           string(source),
		RuntimeRevision:  revision,
	}
}

// trustedReviewedRuntime derives the current trusted managed identity without
// resolving a release catalogue. It mirrors the preview state used to issue
// review evidence and never executes an update command.
func (c *Controller) trustedReviewedRuntime(
	ctx context.Context,
	name string,
) (*dto.AgentRuntimeReviewDTO, agents.ManagedNPMRuntimeSpec, error) {
	ag, ok := c.agentRegistry.Get(name)
	if !ok {
		return nil, agents.ManagedNPMRuntimeSpec{}, ErrAgentNotFound
	}
	if _, harness := ag.(agents.HarnessUpdateAgent); harness {
		return nil, agents.ManagedNPMRuntimeSpec{}, ErrRuntimeReviewInvalid
	}
	spec, fallback, err := c.managedRuntimeUpdateSpec(ag)
	var family managedruntime.OpenCodeFamily
	var source managedruntime.OpenCodeSource
	var revision uint64
	var active string
	if managed, ok := ag.(agents.ManagedNPMRuntimeAgent); ok && name == agents.OpenCodeACPAgentID && c.hasOpenCodeSelection(ctx) {
		spec, family, source, revision, active, _, err = c.managedRuntimeState(ctx, name, managed)
		fallback = false
	}
	if err != nil {
		return nil, agents.ManagedNPMRuntimeSpec{}, err
	}
	if fallback {
		return nil, agents.ManagedNPMRuntimeSpec{}, ErrRuntimeReviewInvalid
	}
	activeVersion, effectiveVersion, defaultVersion := active, "", spec.DefaultVersionOrPinned()
	switch {
	case family == "":
		activeVersion, effectiveVersion, _, err = c.runtimeVersions(ctx, name, spec)
		if err != nil {
			return nil, agents.ManagedNPMRuntimeSpec{}, err
		}
	case active != "":
		effectiveVersion = active
	default:
		effectiveVersion = defaultVersion
	}
	current := managedCurrentVersion(ctx, c.runtimeUpdater, c.managedRuntimeSelections, managedVersionState{
		agentName: name, packageName: spec.Package, fallback: false,
		active: activeVersion, effective: effectiveVersion,
	})
	evidence := reviewedEvidence(spec.Package, current, activeVersion, effectiveVersion, family, source, revision)
	if evidence == nil {
		return nil, spec, ErrRuntimeReviewInvalid
	}
	return evidence, spec, nil
}

func sameReviewedRuntime(left, right *dto.AgentRuntimeReviewDTO) bool {
	if left == nil || right == nil {
		return false
	}
	return left.RuntimeID == right.RuntimeID &&
		left.Package == right.Package &&
		left.CurrentVersion == right.CurrentVersion &&
		left.EffectiveVersion == right.EffectiveVersion &&
		left.ActiveVersion == right.ActiveVersion &&
		left.Family == right.Family &&
		left.Source == right.Source &&
		left.RuntimeRevision == right.RuntimeRevision
}

// verifyReviewedRuntimeLocked compares caller evidence with the trusted runtime
// while runtimeAutoUpdateMu is held. The target must still classify as an
// ordinary update, never rollback, repair, migration, or up to date.
func (c *Controller) verifyReviewedRuntimeLocked(
	ctx context.Context,
	name string,
	targetVersion string,
	evidence dto.AgentRuntimeReviewDTO,
	validateCatalogue bool,
) (agents.ManagedNPMRuntimeSpec, error) {
	current, spec, err := c.trustedReviewedRuntime(ctx, name)
	if err != nil {
		if errors.Is(err, ErrAgentNotFound) || errors.Is(err, ErrRuntimeUpdateUnsupported) {
			return spec, ErrRuntimeReviewChanged
		}
		return spec, err
	}
	if !sameReviewedRuntime(current, &evidence) {
		return spec, ErrRuntimeReviewChanged
	}
	operation, err := managedruntime.ClassifyEffectiveOperation(
		false, current.ActiveVersion, current.EffectiveVersion, current.CurrentVersion,
		targetVersion, spec.DefaultVersionOrPinned(),
	)
	if err != nil || operation != managedruntime.OperationUpdate {
		if err != nil {
			return spec, ErrRuntimeReviewInvalid
		}
		return spec, ErrRuntimeReviewChanged
	}
	if validateCatalogue {
		if err := c.validateAgentUpdateTarget(ctx, spec, targetVersion); err != nil {
			return spec, err
		}
	}
	return spec, nil
}

func (c *Controller) reviewedActivationGuard(
	name string,
	targetVersion string,
	evidence dto.AgentRuntimeReviewDTO,
) runtimeActivationGuard {
	return func(ctx context.Context, activate func() error) error {
		c.runtimeAutoUpdateMu.Lock()
		defer c.runtimeAutoUpdateMu.Unlock()
		if _, err := c.verifyReviewedRuntimeLocked(ctx, name, targetVersion, evidence, false); err != nil {
			return err
		}
		return activate()
	}
}

// EnqueueReviewedAgentUpdate admits a pinned managed update only when the
// caller echoes current, preview-issued runtime evidence. The evidence is
// checked again under the policy lock around activation.
func (c *Controller) EnqueueReviewedAgentUpdate(
	ctx context.Context,
	name string,
	targetVersion string,
	evidence dto.AgentRuntimeReviewDTO,
) (*dto.AgentUpdateJobDTO, error) {
	if c.updateJobStore == nil || c.runtimeUpdater == nil || !c.verifiedManagedActivation() {
		return nil, ErrRuntimeUpdaterUnavailable
	}
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" {
		return nil, ErrRuntimeUpdateTargetRequired
	}
	c.runtimeAutoUpdateMu.Lock()
	defer c.runtimeAutoUpdateMu.Unlock()
	if active, found := c.updateJobStore.GetActive(name); found {
		return nil, &MaintenanceConflictError{
			AgentName: name,
			Active:    MaintenanceJobRef{JobID: active.JobID, Kind: MaintenanceKindUpdate},
		}
	}
	spec, err := c.verifyReviewedRuntimeLocked(ctx, name, targetVersion, evidence, true)
	if err != nil {
		return nil, err
	}
	guard := c.reviewedActivationGuard(name, targetVersion, evidence)
	job, err := c.updateJobStore.enqueueReviewed(name, spec, targetVersion, func() error {
		return c.disableAutomaticUpdatesLocked(ctx, name)
	}, guard)
	if err != nil {
		return nil, err
	}
	if snapshot, found := c.updateJobStore.Get(job.ID); found {
		return snapshot, nil
	}
	snapshot := job.snapshot()
	return &snapshot, nil
}
