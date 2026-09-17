package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type ceilingEntryBindingContextKey struct{}
type ceilingEntryKindContextKey struct{}

const legacyWorkflowEntryIdentity = "legacy:unknown"

// withCeilingEntryBinding carries the immutable workflow-entry identity through
// replay helpers that otherwise use the public launch APIs. The context value
// is only an internal guard; the durable payload remains the source of truth.
func withCeilingEntryBinding(ctx context.Context, binding *models.CeilingWorkflowEntryBinding) context.Context {
	if binding == nil {
		return ctx
	}
	copyOfBinding := *binding
	return context.WithValue(ctx, ceilingEntryBindingContextKey{}, copyOfBinding)
}

func ceilingEntryBindingFromContext(ctx context.Context) *models.CeilingWorkflowEntryBinding {
	if ctx == nil {
		return nil
	}
	binding, ok := ctx.Value(ceilingEntryBindingContextKey{}).(models.CeilingWorkflowEntryBinding)
	if !ok || !binding.Valid() {
		return nil
	}
	return &binding
}

func withCeilingEntryKind(ctx context.Context, kind models.CeilingLaunchKind) context.Context {
	return context.WithValue(ctx, ceilingEntryKindContextKey{}, kind)
}

func ceilingEntryKindFromContext(ctx context.Context) models.CeilingLaunchKind {
	if ctx == nil {
		return ""
	}
	kind, _ := ctx.Value(ceilingEntryKindContextKey{}).(models.CeilingLaunchKind)
	return kind
}

type ceilingEntryDisposition int

const (
	ceilingEntryValid ceilingEntryDisposition = iota
	ceilingEntrySuperseded
	ceilingEntryUnavailable
)

// ErrCeilingLaunchSuperseded is returned after a replay claim when the
// workflow entry or destination changed before runtime admission. The caller
// gives the old record a terminal disposition instead of retargeting it.
var ErrCeilingLaunchSuperseded = fmt.Errorf("ceiling deferred workflow entry was superseded")

func cloneCeilingPayload(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return map[string]interface{}{}
	}
	clone := make(map[string]interface{}, len(payload)+1)
	for key, value := range payload {
		clone[key] = value
	}
	return clone
}

func ceilingEntryBindingValue(binding models.CeilingWorkflowEntryBinding) map[string]interface{} {
	value := map[string]interface{}{
		"workflow_id":         binding.WorkflowID,
		"destination_step_id": binding.DestinationStepID,
		"route_operation_id":  binding.RouteOperationID,
		"entry_identity":      binding.EntryIdentity,
	}
	if binding.DestinationSessionID != "" {
		value["destination_session_id"] = binding.DestinationSessionID
	}
	return value
}

// deriveCeilingEntryBinding obtains the binding from the task's committed or
// prepared route, or from the immutable workflow-step entry identity when the
// route is not materialized yet. It deliberately does not consult workflow
// history: the route or transition ledger is the task-owned admission contract
// and is the only identity replay may safely compare.
//
//nolint:cyclop,funlen // Binding resolution validates independent route, workflow, and recipient identities.
func (s *Service) deriveCeilingEntryBinding(
	ctx context.Context,
	task *models.Task,
	payload map[string]interface{},
	sessionID string,
) (models.CeilingWorkflowEntryBinding, bool) {
	if task == nil {
		return models.CeilingWorkflowEntryBinding{}, false
	}
	if binding, present, err := models.ReadCeilingWorkflowEntryBinding(payload); err != nil {
		return models.CeilingWorkflowEntryBinding{}, false
	} else if present {
		return binding, true
	}

	requestedStepID := stringField(payload, metaKeyWorkflowStepID)
	if requestedStepID == "" {
		requestedStepID = task.WorkflowStepID
	}
	if requestedStepID == "" {
		return models.CeilingWorkflowEntryBinding{}, false
	}

	route, ok := models.LoadWorkflowSessionRoute(task.Metadata)
	if ok && (route.EntryIdentity == "" || route.OperationID == "" || route.DestinationStepID != requestedStepID) {
		return models.CeilingWorkflowEntryBinding{}, false
	}

	var step *wfmodels.WorkflowStep
	if s.workflowStepGetter != nil {
		step, _ = s.workflowStepGetter.GetStep(ctx, requestedStepID)
		if step == nil {
			return models.CeilingWorkflowEntryBinding{}, false
		}
	}
	workflowID := task.WorkflowID
	if step != nil && step.WorkflowID != "" {
		workflowID = step.WorkflowID
	}
	if workflowID == "" {
		return models.CeilingWorkflowEntryBinding{}, false
	}

	entryID := int64Field(payload, "workflow_entry_id")
	entryIdentity := s.workflowEntryIdentity(ctx, task.ID, entryID)
	if ok {
		// A route already carries the authoritative identity. A supplied
		// transition id must still agree with it, otherwise this payload belongs
		// to a different entry than the route currently committed on the task.
		if entryID > 0 && route.EntryIdentity != entryIdentity {
			return models.CeilingWorkflowEntryBinding{}, false
		}
		entryIdentity = route.EntryIdentity
	}
	if entryIdentity == legacyWorkflowEntryIdentity {
		return models.CeilingWorkflowEntryBinding{}, false
	}

	destinationSessionID := sessionID
	if ok && route.DestinationID != "" {
		destinationSessionID = route.DestinationID
	}
	if expectedSessionID := sessionIDFromPayload(payload); expectedSessionID != "" {
		if destinationSessionID != "" && destinationSessionID != expectedSessionID {
			return models.CeilingWorkflowEntryBinding{}, false
		}
		destinationSessionID = expectedSessionID
	}

	operationID := ""
	if ok {
		operationID = route.OperationID
	} else {
		var target *wfmodels.WorkflowSessionTarget
		if step != nil {
			target = step.SessionTarget
		}
		operationID = workflowSessionRouteID(
			task.ID,
			requestedStepID,
			entryIdentity,
			target,
			s.resolveStepProfileSessionStartPolicy(step),
		)
	}
	return models.CeilingWorkflowEntryBinding{
		WorkflowID:           workflowID,
		DestinationStepID:    requestedStepID,
		RouteOperationID:     operationID,
		EntryIdentity:        entryIdentity,
		DestinationSessionID: destinationSessionID,
	}, true
}

// workflowEntryBindingForStep creates the binding used by workflow entry
// dispatch before a session route exists. A transition entry id is preferred;
// the repository's latest transition or the task creation stamp is the bounded
// fallback for legacy/manual entry paths.
func (s *Service) workflowEntryBindingForStep(
	ctx context.Context,
	taskID string,
	step *wfmodels.WorkflowStep,
	sessionID string,
	entryIDs ...int64,
) (*models.CeilingWorkflowEntryBinding, bool) {
	if taskID == "" || step == nil || step.ID == "" || step.WorkflowID == "" {
		return nil, false
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return nil, false
	}
	payload := map[string]interface{}{
		metaKeyWorkflowStepID: step.ID,
	}
	if len(entryIDs) > 0 && entryIDs[0] > 0 {
		payload["workflow_entry_id"] = entryIDs[0]
	}
	binding, ok := s.deriveCeilingEntryBinding(ctx, task, payload, sessionID)
	if !ok {
		return nil, false
	}
	return &binding, true
}

func sessionIDFromPayload(payload map[string]interface{}) string {
	return stringField(payload, metaKeySessionID)
}

// ceilingDeferralTargetsSession resolves the exact recipient of a deferred
// launch when a caller already has a session identity. Session-backed launch
// kinds carry that identity in their payload. Seam 1's sessionless start kind
// instead obtains it from the committed workflow route, when one exists. A
// caller must not claim a sessionless record merely because it happens to be
// the only record on the task: that would let Send Now consume a successor or
// an unrelated start after the route changed.
func (s *Service) ceilingDeferralTargetsSession(
	ctx context.Context,
	taskID string,
	deferral models.CeilingDeferral,
	expectedSessionID string,
) (bool, error) {
	if expectedSessionID == "" {
		return true, nil
	}
	if destinationID := sessionIDFromCeilingPayload(deferral); destinationID != "" {
		return destinationID == expectedSessionID, nil
	}
	if deferral.Kind != models.CeilingLaunchStart {
		return false, nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return false, err
	}
	if task == nil {
		return false, nil
	}
	route, routePresent := models.LoadWorkflowSessionRoute(task.Metadata)
	if !routePresent || route.DestinationID == "" {
		return false, nil
	}
	if binding, present, bindingErr := models.ReadCeilingWorkflowEntryBinding(deferral.Payload); bindingErr != nil {
		return false, nil
	} else if present &&
		(binding.DestinationSessionID != "" && binding.DestinationSessionID != route.DestinationID ||
			binding.RouteOperationID != route.OperationID ||
			binding.DestinationStepID != route.DestinationStepID ||
			binding.EntryIdentity != route.EntryIdentity) {
		return false, nil
	}
	return route.DestinationID == expectedSessionID, nil
}

// enrichCeilingLaunchPayload adds a route binding when the route is already
// available. Failure to derive one is intentionally non-fatal for a generic
// launch; replay then applies the legacy ambiguity guard.
func (s *Service) enrichCeilingLaunchPayload(
	ctx context.Context,
	taskID string,
	sessionID string,
	payload map[string]interface{},
) map[string]interface{} {
	if _, present, err := models.ReadCeilingWorkflowEntryBinding(payload); err != nil {
		return payload
	} else if present {
		return payload
	}
	if binding := ceilingEntryBindingFromContext(ctx); binding != nil {
		updated := cloneCeilingPayload(payload)
		updated[models.CeilingLaunchEntryBindingKey] = ceilingEntryBindingValue(*binding)
		return updated
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return payload
	}
	binding, ok := s.deriveCeilingEntryBinding(ctx, task, payload, sessionID)
	if !ok {
		return payload
	}
	updated := cloneCeilingPayload(payload)
	updated[models.CeilingLaunchEntryBindingKey] = ceilingEntryBindingValue(binding)
	return updated
}

// enrichCeilingDeferralBinding performs a conditional payload-only update for
// legacy records. Queue timestamp and all other record identity remain exactly
// as stored. A concurrent successor wins the CAS and is returned to the caller
// for fresh validation.
//
//nolint:cyclop,nestif // Binding enrichment keeps compare-and-set ownership adjacent to the legacy fallback.
func (s *Service) enrichCeilingDeferralBinding(
	ctx context.Context,
	task *models.Task,
	deferral models.CeilingDeferral,
) (models.CeilingDeferral, error) {
	binding, present, err := models.ReadCeilingWorkflowEntryBinding(deferral.Payload)
	if err != nil {
		return deferral, nil
	}
	needsWrite := false
	if present {
		// A seam-1 record may be written before an explicit session-target
		// route commits its destination. Complete the optional destination id
		// once that route is durable, without changing the entry identity.
		if task != nil {
			if route, routePresent := models.LoadWorkflowSessionRoute(task.Metadata); routePresent &&
				route.OperationID == binding.RouteOperationID &&
				route.DestinationStepID == binding.DestinationStepID &&
				route.EntryIdentity == binding.EntryIdentity &&
				route.DestinationID != "" && binding.DestinationSessionID == "" {
				binding.DestinationSessionID = route.DestinationID
				needsWrite = true
			}
		}
		if !needsWrite {
			return deferral, nil
		}
	} else {
		binding, present = s.deriveCeilingEntryBinding(ctx, task, deferral.Payload, sessionIDFromCeilingPayload(deferral))
		if !present {
			return deferral, nil
		}
	}
	for attempt := 0; attempt < deferredLaunchCASRetryBudget; attempt++ {
		record, prior, err := s.repo.GetTaskDeferredLaunch(ctx, task.ID)
		if err != nil {
			return deferral, err
		}
		current, err := models.ReadCeilingDeferral(record)
		if err != nil {
			return deferral, nil
		}
		equivalent, err := sameCeilingDeferralIdentity(current, deferral)
		if err != nil {
			return deferral, err
		}
		if !equivalent {
			return deferral, fmt.Errorf("%w: deferred workflow entry changed while binding", ErrCeilingLaunchSuperseded)
		}
		updated := cloneCeilingRecord(record)
		updatedPayload := cloneCeilingPayload(current.Payload)
		updatedPayload[models.CeilingLaunchEntryBindingKey] = ceilingEntryBindingValue(binding)
		updated[models.CeilingLaunchPayloadKey] = updatedPayload
		stored, lostCompare, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, task.ID, prior, updated)
		if err != nil {
			return deferral, err
		}
		if stored {
			current.Payload = updatedPayload
			return current, nil
		}
		if !lostCompare {
			return deferral, fmt.Errorf("repository reported no binding update")
		}
	}
	return deferral, fmt.Errorf("workflow entry binding update retries exhausted")
}

// ceilingDeferralsEquivalentForAdmission keeps a legacy record without the
// optional workflow binding equivalent to the same launch after the binding
// has become available. Once both records carry a binding, the binding is part
// of the identity and must compare exactly.
func ceilingDeferralsEquivalentForAdmission(a, b models.CeilingDeferral) (bool, error) {
	left := cloneCeilingPayload(a.Payload)
	right := cloneCeilingPayload(b.Payload)
	_, leftBound := left[models.CeilingLaunchEntryBindingKey]
	_, rightBound := right[models.CeilingLaunchEntryBindingKey]
	if leftBound != rightBound {
		delete(left, models.CeilingLaunchEntryBindingKey)
		delete(right, models.CeilingLaunchEntryBindingKey)
	}
	a.Payload = left
	b.Payload = right
	return models.CeilingDeferralsEquivalent(a, b)
}

// validateCeilingEntry compares a claimed record with the current task-owned
// route and destination. It is deliberately read-only. Callers retain the
// record on unavailable reads and only terminally dispose superseded entries.
func (s *Service) validateCeilingEntry(
	ctx context.Context,
	task *models.Task,
	deferral models.CeilingDeferral,
) (ceilingEntryDisposition, string, error) {
	return s.validateCeilingEntryWithDestinationState(ctx, task, deferral, true)
}

// validateCeilingEntryWithDestinationState checks the task-owned workflow
// identity and, when requested, the destination's pre-dispatch state. The
// full check is used while a replay still owns a queued record. Dispatch
// helpers also carry the binding into their call graph, but those helpers may
// observe the destination as RUNNING after this launch has claimed it; they
// still need the route/entry guard without mistaking their own state change
// for a competing launch.
//
//nolint:cyclop,gocognit,nestif // Replay admission checks independent task, route, workflow, and destination invariants.
func (s *Service) validateCeilingEntryWithDestinationState(
	ctx context.Context,
	task *models.Task,
	deferral models.CeilingDeferral,
	checkDestinationState bool,
) (ceilingEntryDisposition, string, error) {
	if task == nil {
		return ceilingEntryUnavailable, "task is unavailable", nil
	}
	binding, present, err := models.ReadCeilingWorkflowEntryBinding(deferral.Payload)
	if err != nil {
		return ceilingEntryUnavailable, err.Error(), nil
	}
	route, routePresent := models.LoadWorkflowSessionRoute(task.Metadata)
	payloadStepID := stringField(deferral.Payload, metaKeyWorkflowStepID)
	workflowEntryID := int64Field(deferral.Payload, "workflow_entry_id")
	workflowOrigin := present || payloadStepID != "" || workflowEntryID > 0
	if !present && routePresent && (deferral.Kind == models.CeilingLaunchStart ||
		(deferral.Kind == models.CeilingLaunchStartCreated && sessionIDFromCeilingPayload(deferral) != "")) {
		workflowOrigin = true
	}
	if !present {
		if !workflowOrigin {
			return s.validateCeilingDestination(ctx, task, deferral, models.CeilingWorkflowEntryBinding{}), "", nil
		}
		if !routePresent || route.EntryIdentity == "" {
			return ceilingEntryUnavailable, "workflow entry route is unavailable", nil
		}
		if payloadStepID != "" && route.DestinationStepID != payloadStepID {
			return ceilingEntrySuperseded, "workflow destination step changed", nil
		}
		if payloadSessionID := sessionIDFromCeilingPayload(deferral); payloadSessionID != "" &&
			route.DestinationID != "" && route.DestinationID != payloadSessionID {
			return ceilingEntrySuperseded, "workflow destination session changed", nil
		}
		// A legacy sessionless start has no recipient identity to compare with
		// the current route. Replaying it against whichever destination happens
		// to be current would retarget an old workflow entry, so only the newer
		// bound form is eligible for this shape.
		if deferral.Kind == models.CeilingLaunchStart && sessionIDFromCeilingPayload(deferral) == "" {
			return ceilingEntryUnavailable, "legacy sessionless workflow entry is ambiguous", nil
		}
		derivedBinding, derived := s.deriveCeilingEntryBinding(ctx, task, deferral.Payload, sessionIDFromCeilingPayload(deferral))
		if !derived {
			return ceilingEntryUnavailable, "workflow entry binding is ambiguous", nil
		}
		binding = derivedBinding
	}
	if !binding.Valid() {
		return ceilingEntryUnavailable, "workflow entry binding is incomplete", nil
	}
	if routePresent {
		if route.OperationID != binding.RouteOperationID ||
			route.DestinationStepID != binding.DestinationStepID ||
			route.EntryIdentity != binding.EntryIdentity ||
			(route.DestinationID != "" && binding.DestinationSessionID != "" && route.DestinationID != binding.DestinationSessionID) {
			return ceilingEntrySuperseded, "workflow destination route changed", nil
		}
	} else if task.WorkflowStepID != binding.DestinationStepID &&
		deferral.Kind != models.CeilingLaunchWorkflowStepEnsure {
		return ceilingEntrySuperseded, "workflow destination step changed", nil
	}
	if task.WorkflowID != "" && task.WorkflowID != binding.WorkflowID {
		return ceilingEntrySuperseded, "workflow destination workflow changed", nil
	}
	currentEntryIdentity := s.workflowEntryIdentity(ctx, task.ID)
	if currentEntryIdentity != legacyWorkflowEntryIdentity && binding.EntryIdentity != currentEntryIdentity {
		return ceilingEntrySuperseded, "workflow entry identity changed", nil
	}
	if s.workflowStepGetter != nil {
		step, stepErr := s.workflowStepGetter.GetStep(ctx, binding.DestinationStepID)
		if stepErr != nil {
			return ceilingEntryUnavailable, "workflow destination step could not be read", stepErr
		}
		// Some legacy workflow-step providers return the step without its
		// parent workflow id. The task/route binding remains authoritative in
		// that case; only a populated, conflicting provider id supersedes it.
		if step == nil || (step.WorkflowID != "" && step.WorkflowID != binding.WorkflowID) {
			return ceilingEntrySuperseded, "workflow destination workflow changed", nil
		}
	}
	return s.validateCeilingDestination(ctx, task, deferral, binding, checkDestinationState), "", nil
}

func (s *Service) validateCeilingDestination(
	ctx context.Context,
	task *models.Task,
	deferral models.CeilingDeferral,
	binding models.CeilingWorkflowEntryBinding,
	checkDestinationState ...bool,
) ceilingEntryDisposition {
	destinationID := binding.DestinationSessionID
	if destinationID == "" {
		destinationID = sessionIDFromCeilingPayload(deferral)
	}
	if destinationID == "" {
		return ceilingEntryValid
	}
	session, err := s.repo.GetTaskSession(ctx, destinationID)
	if err != nil || session == nil {
		if err != nil {
			return ceilingEntryUnavailable
		}
		return ceilingEntrySuperseded
	}
	if session.TaskID != task.ID || isTerminalSessionState(session.State) {
		return ceilingEntrySuperseded
	}
	// A record waiting for capacity must never replay into a provider that has
	// already started. That would duplicate a workflow prompt or continuation.
	// Dispatch-time binding checks can opt out after this launch has itself
	// claimed RUNNING; the task-owned route and session identity remain guarded.
	if len(checkDestinationState) == 0 || checkDestinationState[0] {
		if session.State == models.TaskSessionStateStarting || session.State == models.TaskSessionStateRunning {
			return ceilingEntrySuperseded
		}
	}
	return ceilingEntryValid
}

func (s *Service) validateClaimedCeilingBinding(
	ctx context.Context,
	taskID string,
	binding *models.CeilingWorkflowEntryBinding,
) error {
	if binding == nil {
		return nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("reload task for deferred workflow entry: %w", err)
	}
	if task == nil {
		return ErrCeilingLaunchSuperseded
	}
	kind := ceilingEntryKindFromContext(ctx)
	if kind == "" {
		kind = models.CeilingLaunchStartCreated
	}
	disposition, detail, validationErr := s.validateCeilingEntryWithDestinationState(ctx, task, models.CeilingDeferral{
		Kind: kind,
		Payload: map[string]interface{}{
			models.CeilingLaunchEntryBindingKey: ceilingEntryBindingValue(*binding),
			metaKeySessionID:                    binding.DestinationSessionID,
		},
	}, false)
	if validationErr != nil {
		return validationErr
	}
	if disposition == ceilingEntrySuperseded {
		if detail == "" {
			return ErrCeilingLaunchSuperseded
		}
		return fmt.Errorf("%w: %s", ErrCeilingLaunchSuperseded, detail)
	}
	if disposition == ceilingEntryUnavailable {
		return fmt.Errorf("deferred workflow entry ownership is unavailable")
	}
	return nil
}

func (s *Service) validateContextCeilingEntry(ctx context.Context, taskID string) error {
	return s.validateClaimedCeilingBinding(ctx, taskID, ceilingEntryBindingFromContext(ctx))
}
