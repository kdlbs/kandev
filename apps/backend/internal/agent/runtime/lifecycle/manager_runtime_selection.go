package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// Safe runtime-selection reason codes. They are stable, bounded identifiers
// that may cross the launch-failure boundary; raw provider errors never do.
const (
	ExecutorRuntimeReasonUnavailable        = "executor_runtime_unavailable"
	ExecutorRuntimeReasonMismatch           = "executor_runtime_mismatch"
	ExecutorRuntimeReasonIdentityUnverified = "executor_identity_unverified"
)

var (
	// ErrExecutorRuntimeUnavailable is the sentinel for a concrete selection
	// whose mapped backend is not registered.
	ErrExecutorRuntimeUnavailable = errors.New("executor runtime unavailable")
	// ErrExecutorRuntimeMismatch is the sentinel for a retained execution whose
	// actual runtime does not satisfy the admitted selection.
	ErrExecutorRuntimeMismatch = errors.New("executor runtime mismatch")
	// ErrExecutorIdentityUnverified is the sentinel for a Docker execution whose
	// container identity is missing or contradictory.
	ErrExecutorIdentityUnverified = errors.New("executor identity unverified")
)

// ExecutorRuntimeError carries the safe, bounded runtime-selection diagnostic.
// It deliberately omits endpoints, host paths, secret references, profile
// details, and raw environment contents.
type ExecutorRuntimeError struct {
	Reason       string
	ExecutorType string
	Runtime      executor.Name
	Observed     executor.Name
	Detail       string
	// EnvironmentMismatch distinguishes a retained execution bound to another
	// task environment from a runtime-versus-runtime contradiction.
	EnvironmentMismatch bool
}

// IsEnvironmentMismatch reports whether this refusal is an environment-binding
// mismatch rather than an observed-runtime mismatch.
func (e *ExecutorRuntimeError) IsEnvironmentMismatch() bool {
	return e != nil && e.EnvironmentMismatch
}

func (e *ExecutorRuntimeError) Error() string {
	if e == nil {
		return ""
	}
	selected := string(e.Runtime)
	if selected == "" {
		selected = strings.TrimSpace(e.ExecutorType)
	}
	switch e.Reason {
	case ExecutorRuntimeReasonMismatch:
		if e.Detail != "" {
			return fmt.Sprintf(
				"selected executor runtime %q does not match the retained execution: %s",
				selected, e.Detail,
			)
		}
		return fmt.Sprintf(
			"selected executor runtime %q does not match observed runtime %q",
			selected, string(e.Observed),
		)
	case ExecutorRuntimeReasonIdentityUnverified:
		if e.Detail != "" {
			return fmt.Sprintf(
				"selected executor runtime %q has no verifiable runner identity: %s",
				selected, e.Detail,
			)
		}
		return fmt.Sprintf("selected executor runtime %q has no verifiable runner identity", selected)
	default:
		if e.Detail != "" {
			return fmt.Sprintf("selected executor runtime %q is unavailable: %s", selected, e.Detail)
		}
		return fmt.Sprintf("selected executor runtime %q is unavailable", selected)
	}
}

func (e *ExecutorRuntimeError) Unwrap() error {
	if e == nil {
		return nil
	}
	switch e.Reason {
	case ExecutorRuntimeReasonMismatch:
		return ErrExecutorRuntimeMismatch
	case ExecutorRuntimeReasonIdentityUnverified:
		return ErrExecutorIdentityUnverified
	default:
		return ErrExecutorRuntimeUnavailable
	}
}

// IsExecutorRuntimeRefusal reports whether err is a typed runtime-selection
// refusal. Callers use it to preserve selection and observation rather than
// entering stale-runtime teardown or replacement.
func IsExecutorRuntimeRefusal(err error) bool {
	return errors.Is(err, ErrExecutorRuntimeUnavailable) ||
		errors.Is(err, ErrExecutorRuntimeMismatch) ||
		errors.Is(err, ErrExecutorIdentityUnverified)
}

// normalizeExecutorSelection maps an executor type to its required backend.
// The empty result means the selection is unspecified (existing standalone
// default behavior).
func normalizeExecutorSelection(executorType string) (executor.Name, bool) {
	trimmed := strings.TrimSpace(executorType)
	if trimmed == "" {
		return executor.NameUnknown, false
	}
	return executor.ExecutorTypeToBackend(models.ExecutorType(trimmed)), true
}

func runtimeSatisfiesSelection(selected, observed executor.Name) bool {
	if selected == executor.NameUnknown || observed == executor.NameUnknown {
		return false
	}
	return selected == observed
}

// ValidateExecutorRuntime reports whether the concrete selection's mapped
// backend is registered. It performs no provisioning, credential resolution,
// or workspace work. An empty selection is valid.
func (m *Manager) ValidateExecutorRuntime(executorType string) error {
	selected, required := normalizeExecutorSelection(executorType)
	if !required {
		return nil
	}
	failure := &ExecutorRuntimeError{
		Reason:       ExecutorRuntimeReasonUnavailable,
		ExecutorType: strings.TrimSpace(executorType),
		Runtime:      selected,
	}
	if m.executorRegistry == nil {
		failure.Detail = "runtime registry is not configured"
		return failure
	}
	if selected == executor.NameUnknown {
		failure.Detail = "runtime is unknown"
		return failure
	}
	if _, err := m.executorRegistry.GetBackend(selected); err != nil {
		failure.Detail = "runtime is disabled or not registered"
		return failure
	}
	return nil
}

// ValidateExecutionRuntime compares a retained execution's recorded runtime
// with the admitted selection for the requesting session and environment.
// It is read-only: it never stops, migrates, relabels, or replaces a worker.
func (m *Manager) ValidateExecutionRuntime(
	ctx context.Context,
	sessionID string,
	taskEnvironmentID string,
	executorType string,
) error {
	var execution *AgentExecution
	if sessionID != "" && m.executionStore != nil {
		execution, _ = m.executionStore.GetBySessionID(sessionID)
	}
	selected, required := normalizeExecutorSelection(executorType)
	if !required && execution != nil && strings.TrimSpace(execution.ExecutorType) != "" {
		// An authenticated retained binding is selection evidence even when a
		// legacy request omits its executor type.
		selected, required = normalizeExecutorSelection(execution.ExecutorType)
	}
	if !required {
		return nil
	}
	if execution == nil {
		return m.validateDurableExecutionRuntime(ctx, sessionID, taskEnvironmentID, selected)
	}
	return m.validateRetainedExecutionRuntime(ctx, execution, selected, taskEnvironmentID)
}

func (m *Manager) validateRetainedExecutionRuntime(
	ctx context.Context,
	execution *AgentExecution,
	selected executor.Name,
	taskEnvironmentID string,
) error {
	if execution == nil {
		return nil
	}
	if taskEnvironmentID != "" && execution.TaskEnvironmentID != "" &&
		execution.TaskEnvironmentID != taskEnvironmentID {
		return &ExecutorRuntimeError{
			Reason:       ExecutorRuntimeReasonMismatch,
			ExecutorType: execution.ExecutorType,
			Runtime:      selected,
			Observed:     execution.RuntimeName,
			Detail: fmt.Sprintf(
				"retained execution belongs to task environment %q, not %q",
				execution.TaskEnvironmentID, taskEnvironmentID,
			),
			EnvironmentMismatch: true,
		}
	}
	if err := validateObservedRuntime(execution.ExecutorType, selected, execution.RuntimeName); err != nil {
		if !errors.Is(err, ErrExecutorIdentityUnverified) {
			return err
		}
		return m.validateFallbackRetainedIdentity(ctx, execution, selected)
	}
	return m.validateRetainedIdentity(ctx, execution, selected)
}

// validateFallbackRetainedIdentity resolves an in-memory execution whose
// observed runtime is empty through its durable row when that row can supply
// independent identity.
func (m *Manager) validateFallbackRetainedIdentity(
	ctx context.Context,
	execution *AgentExecution,
	selected executor.Name,
) error {
	row, available, readErr := m.readRetainedInventory(ctx, execution.SessionID)
	if readErr != nil {
		return identityUnverifiedError(execution.ExecutorType, selected, "retained runtime inventory could not be read")
	}
	if !available || row == nil {
		if !identityRequiredForSelection(selected, execution) {
			return nil
		}
		return identityUnverifiedError(execution.ExecutorType, selected, "retained runtime identity is unavailable")
	}
	return validateRetainedRow(execution, selected, row)
}

// readRetainedInventory distinguishes authoritative not-found from a failed
// read. available=false means no durable reader is configured.
func (m *Manager) readRetainedInventory(ctx context.Context, sessionID string) (*models.ExecutorRunning, bool, error) {
	if sessionID == "" {
		return nil, false, nil
	}
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return nil, false, nil
	}
	row, err := reader.GetExecutorRunningBySessionID(ctx, sessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil, true, nil
	}
	if err != nil {
		return nil, true, err
	}
	return row, true, nil
}

func identityUnverifiedError(executorType string, selected executor.Name, detail string) error {
	return &ExecutorRuntimeError{
		Reason:       ExecutorRuntimeReasonIdentityUnverified,
		ExecutorType: strings.TrimSpace(executorType),
		Runtime:      selected,
		Detail:       detail,
	}
}

// identityRequiredForSelection keeps a Docker boundary fail-closed while
// allowing a live standalone execution to recover when its local runtime row
// was removed during process replacement and its observed runtime is missing.
func identityRequiredForSelection(selected executor.Name, execution *AgentExecution) bool {
	if selected == executor.NameDocker {
		return true
	}
	if selected != executor.NameStandalone {
		return true
	}
	// A standalone boundary has no container identity to prove. A missing
	// runtime on a live execution is expected while its local process is
	// replaced. A container ID is contradictory evidence and stays refused.
	if execution == nil {
		return false
	}
	return strings.TrimSpace(execution.ContainerID) != ""
}

func validateRetainedRow(execution *AgentExecution, selected executor.Name, row *models.ExecutorRunning) error {
	if row == nil || row.Runtime == "" {
		if !identityRequiredForSelection(selected, execution) {
			return nil
		}
		return identityUnverifiedError(execution.ExecutorType, selected, "retained runtime identity is unavailable")
	}
	if row.Runtime != selected {
		return &ExecutorRuntimeError{
			Reason:       ExecutorRuntimeReasonMismatch,
			ExecutorType: execution.ExecutorType,
			Runtime:      selected,
			Observed:     row.Runtime,
		}
	}
	if execution.ID != "" && row.AgentExecutionID != "" && row.AgentExecutionID != execution.ID {
		return identityUnverifiedError(execution.ExecutorType, selected, "retained execution identity does not match")
	}
	if selected == executor.NameDocker {
		executionContainer := strings.TrimSpace(execution.ContainerID)
		retainedContainer := strings.TrimSpace(row.ContainerID)
		if retainedContainer == "" || (executionContainer != "" && executionContainer != retainedContainer) {
			return identityUnverifiedError(execution.ExecutorType, selected, "retained container identity is unverified")
		}
	}
	return nil
}

// validateDurableExecutionRuntime applies the same comparison when no live
// in-memory execution exists. The durable row describes the last recorded
// execution; it never becomes a liveness claim. A read failure or unknown
// runtime on an existing row is a refusal, never a fresh-session success.
func (m *Manager) validateDurableExecutionRuntime(
	ctx context.Context,
	sessionID string,
	taskEnvironmentID string,
	selected executor.Name,
) error {
	row, available, err := m.readRetainedInventory(ctx, sessionID)
	if err != nil {
		return identityUnverifiedError("", selected, "retained runtime inventory could not be read")
	}
	if !available {
		if m.runningWriter != nil {
			return identityUnverifiedError("", selected, "retained runtime inventory is unavailable")
		}
		return nil
	}
	if row == nil || executorRunningRowIsHistorical(row.Status) {
		return nil
	}
	return validateDurableRow(selected, row, taskEnvironmentID)
}

func validateDurableRow(selected executor.Name, row *models.ExecutorRunning, taskEnvironmentID string) error {
	if retainedRowBelongsToOtherEnvironment(row, taskEnvironmentID) {
		// The row describes the predecessor for an authorized environment
		// transition. It is not a reuse candidate for the selected environment;
		// the transition retires that exact execution through its own cleanup.
		return nil
	}
	if row == nil || row.Runtime == "" {
		if selected == executor.NameStandalone && row != nil && strings.TrimSpace(row.ContainerID) == "" {
			return nil
		}
		return identityUnverifiedError("", selected, "retained runtime identity is unavailable")
	}
	if row.Runtime != selected {
		return &ExecutorRuntimeError{
			Reason:   ExecutorRuntimeReasonMismatch,
			Runtime:  selected,
			Observed: row.Runtime,
		}
	}
	if selected == executor.NameDocker && strings.TrimSpace(row.ContainerID) == "" {
		return identityUnverifiedError("", selected, "retained container identity is unverified")
	}
	return nil
}

func retainedRowBelongsToOtherEnvironment(row *models.ExecutorRunning, taskEnvironmentID string) bool {
	if row == nil || taskEnvironmentID == "" || row.Metadata == nil {
		return false
	}
	rowEnvironmentID, _ := row.Metadata[MetadataKeyTaskEnvironmentID].(string)
	return rowEnvironmentID != "" && rowEnvironmentID != taskEnvironmentID
}

// executorRunningRowIsHistorical reports whether the inventory row describes a
// terminal execution. A stopped row is retained history, not a live worker, so
// it cannot satisfy or contradict a new selected runtime.
func executorRunningRowIsHistorical(status string) bool {
	switch status {
	case models.ExecutorRunningStatusFailed,
		models.ExecutorRunningStatusStopped,
		models.ExecutorRunningStatusComplete:
		return true
	default:
		return false
	}
}

// ensureDurableExecutionSelectionCompatible is the cold-path admission used
// before workspace reconciliation, preparation request construction, or
// backend allocation. evidenceRetained says the caller already knows a
// retained record exists; a missing row is then contradictory, not fresh.
func (m *Manager) ensureDurableExecutionSelectionCompatible(
	ctx context.Context,
	sessionID string,
	taskEnvironmentID string,
	executorType string,
	evidenceRetained bool,
) error {
	selected, required := normalizeExecutorSelection(executorType)
	row, available, err := m.readRetainedInventory(ctx, sessionID)
	if err != nil {
		return identityUnverifiedError(executorType, selected, "retained runtime inventory could not be read")
	}
	if !available {
		// A configured writer that cannot read inventory is a wiring failure;
		// retained evidence then fails closed. No writer at all is an isolated
		// caller with no durable inventory to compare (production always wires
		// one through the repository adapter).
		if evidenceRetained && m.runningWriter != nil {
			return identityUnverifiedError(executorType, selected, "retained runtime inventory is unavailable")
		}
		return nil
	}
	if row == nil {
		if evidenceRetained {
			selected, required := normalizeExecutorSelection(executorType)
			if !required || selected == executor.NameStandalone {
				// A local runtime replacement removes its inventory row while the
				// execution stays in memory. A standalone selection has no container
				// boundary to re-establish and can proceed to re-persist the row.
				return nil
			}
			return identityUnverifiedError(executorType, selected, "retained runtime identity is unavailable")
		}
		return nil
	}
	if executorRunningRowIsHistorical(row.Status) {
		return nil
	}
	if !required {
		// A durable row stores the normalized backend name, not an executor
		// type. It is retained selection evidence when the request omits one.
		if row.Runtime == "" {
			if strings.TrimSpace(row.ContainerID) == "" {
				return nil
			}
			return identityUnverifiedError(executorType, executor.NameUnknown, "retained runtime identity is unavailable")
		}
		selected, required = row.Runtime, true
	}
	if !required {
		return nil
	}
	return validateDurableRow(selected, row, taskEnvironmentID)
}

func validateObservedRuntime(executorType string, selected, observed executor.Name) error {
	if observed == "" {
		return &ExecutorRuntimeError{
			Reason:       ExecutorRuntimeReasonIdentityUnverified,
			ExecutorType: strings.TrimSpace(executorType),
			Runtime:      selected,
		}
	}
	if !runtimeSatisfiesSelection(selected, observed) {
		return &ExecutorRuntimeError{
			Reason:       ExecutorRuntimeReasonMismatch,
			ExecutorType: strings.TrimSpace(executorType),
			Runtime:      selected,
			Observed:     observed,
		}
	}
	return nil
}

func (m *Manager) validateRetainedIdentity(ctx context.Context, execution *AgentExecution, selected executor.Name) error {
	containerID := strings.TrimSpace(execution.ContainerID)
	if selected == executor.NameDocker && containerID == "" {
		return identityUnverifiedError(execution.ExecutorType, selected, "retained container identity is unverified")
	}
	row, available, err := m.readRetainedInventory(ctx, execution.SessionID)
	if err != nil {
		return identityUnverifiedError(execution.ExecutorType, selected, "retained runtime inventory could not be read")
	}
	if !available {
		return nil
	}
	if row == nil {
		// An in-memory execution already carries its observed runtime. A missing
		// inventory row is expected while a local runtime is replaced; require
		// durable evidence only when the selected boundary needs it.
		if identityRequiredForSelection(selected, execution) {
			return identityUnverifiedError(execution.ExecutorType, selected, "retained runtime identity is unavailable")
		}
		return nil
	}
	return validateRetainedRow(execution, selected, row)
}

// validateCreatedExecutionIdentity verifies that the backend that created the
// instance agrees with the instance's advertised runtime, and that a Docker
// selection produced a separate container identity before success is
// published. The hostname and filesystem markers are never identity evidence.
func validateCreatedExecutionIdentity(
	executorType string,
	backend ExecutorBackend,
	instance *ExecutorInstance,
) error {
	if backend == nil || instance == nil {
		return nil
	}
	selected, required := normalizeExecutorSelection(executorType)
	if !required {
		return nil
	}
	observed := instance.RuntimeName
	if observed == "" {
		observed = backend.Name()
	}
	if !runtimeSatisfiesSelection(selected, observed) || backend.Name() != selected {
		return &ExecutorRuntimeError{
			Reason:       ExecutorRuntimeReasonMismatch,
			ExecutorType: strings.TrimSpace(executorType),
			Runtime:      selected,
			Observed:     observed,
		}
	}
	if selected == executor.NameDocker && strings.TrimSpace(instance.ContainerID) == "" {
		return &ExecutorRuntimeError{
			Reason:       ExecutorRuntimeReasonIdentityUnverified,
			ExecutorType: strings.TrimSpace(executorType),
			Runtime:      selected,
			Observed:     observed,
		}
	}
	return nil
}

// requireRuntimeBackend resolves exactly the mapped backend for a nonempty
// selection. It is the single admission point used by every launch path.
func (m *Manager) requireRuntimeBackend(executorType string) (ExecutorBackend, error) {
	failure := &ExecutorRuntimeError{
		Reason:       ExecutorRuntimeReasonUnavailable,
		ExecutorType: strings.TrimSpace(executorType),
	}
	selected, required := normalizeExecutorSelection(executorType)
	failure.Runtime = selected
	if m.executorRegistry == nil {
		failure.Detail = "runtime registry is not configured"
		return nil, failure
	}
	if !required {
		return m.executorRegistry.GetBackend(executor.NameStandalone)
	}
	if selected == executor.NameUnknown {
		failure.Detail = "runtime is unknown"
		return nil, failure
	}
	backend, err := m.executorRegistry.GetBackend(selected)
	if err != nil {
		failure.Detail = "runtime is disabled or not registered"
		return nil, failure
	}
	return backend, nil
}

// requireExecutionSelectionCompatibility validates a live in-memory execution
// against its recorded selection before new authenticated effects. An
// unspecified legacy selection is left to existing behavior.
func (m *Manager) requireExecutionSelectionCompatibility(ctx context.Context, execution *AgentExecution) error {
	if execution == nil {
		return nil
	}
	executorType := execution.ExecutorType
	if executorType == "" && execution.MetadataSnapshot() != nil {
		executorType = getMetadataString(execution.MetadataSnapshot(), MetadataKeyExecutorType)
	}
	selected, required := normalizeExecutorSelection(executorType)
	if !required {
		return nil
	}
	return m.validateRetainedExecutionRuntime(ctx, execution, selected, execution.TaskEnvironmentID)
}

// recoveredAvailabilityCompatible suppresses availability publication for a
// recovered execution whose recorded runtime cannot satisfy its selection. The
// authenticated worker remains observable and stoppable; it just cannot
// authorize new prompts.
func (m *Manager) recoveredAvailabilityCompatible(ctx context.Context, execution *AgentExecution) bool {
	err := m.retryRecoveredAvailabilityCompatibility(ctx, execution)
	if err != nil {
		if m.logger != nil {
			m.logger.Warn("suppressing recovered availability for mismatched executor runtime",
				zap.String("execution_id", execution.ID),
				zap.String("session_id", execution.SessionID),
				zap.String("selected_executor_type", execution.ExecutorType),
				zap.String("observed_runtime", string(execution.RuntimeName)),
				zap.Error(err))
		}
		return false
	}
	return true
}

// retryRecoveredAvailabilityCompatibility retries identity-read failures so a
// transient inventory error cannot suppress a recovered execution's only
// Ready event. Genuine mismatches are not retried.
func (m *Manager) retryRecoveredAvailabilityCompatibility(ctx context.Context, execution *AgentExecution) error {
	const attempts = 3
	const retryDelay = 50 * time.Millisecond
	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		err = m.requireExecutionSelectionCompatibility(ctx, execution)
		if err == nil || !errors.Is(err, ErrExecutorIdentityUnverified) || ctx.Err() != nil {
			return err
		}
		if attempt == attempts-1 || !waitForRetainedInventoryRetry(ctx, retryDelay) {
			return err
		}
	}
	return err
}

func waitForRetainedInventoryRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
