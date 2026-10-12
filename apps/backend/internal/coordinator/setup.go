package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
)

// Step ids of the guided setup; a setup 400 names the step that owns the
// failing value.
const (
	setupStepIdentity = "identity"
	setupStepWatches  = "watches"
	setupStepGoal     = "goal"
	setupStepContext  = "context"
	setupStepMayDo    = "may-do"
)

// setupRequest is a setup body that passed the top-level shape check: the
// scalar members are decoded, the object members are kept raw for their
// owners' validators.
type setupRequest struct {
	name, agentProfileID, executorProfileID, context string
	watches, policy, goal                            json.RawMessage
}

// setupPlan is a fully validated setup, ready to insert.
type setupPlan struct {
	name, agentProfileID, executorProfileID, context string
	watches                                          *watchesRequest
	policy                                           Policy
	goal                                             *goalInput
}

func stepError(step string, err error) error {
	var fe *FieldError
	if errors.As(err, &fe) {
		return &SettingsError{Step: step, Field: fe.Field, Message: fe.Message}
	}
	var se *SettingsError
	if errors.As(err, &se) {
		out := *se
		out.Step = step
		return &out
	}
	return err
}

func stringMember(top map[string]json.RawMessage, key string) (string, error) {
	raw, ok := presentMember(top, key)
	if !ok {
		return "", nil
	}
	s, isString := jsonString(raw)
	if !isString {
		return "", bodyErr(key, key+" must be a string")
	}
	return s, nil
}

// decodeSetupRequest checks the body's top-level shape: a JSON object whose
// members have the right JSON type. Nothing here needs a read.
func decodeSetupRequest(body []byte) (setupRequest, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil || top == nil {
		return setupRequest{}, bodyErr("", "body must be a JSON object")
	}
	var req setupRequest
	for _, m := range []struct {
		key string
		dst *string
	}{{PatchFieldName, &req.name}, {PatchFieldAgentProfileID, &req.agentProfileID}, {PatchFieldExecutorProfileID, &req.executorProfileID}, {PatchFieldContext, &req.context}} {
		v, err := stringMember(top, m.key)
		if err != nil {
			return setupRequest{}, err
		}
		*m.dst = v
	}
	req.watches, _ = presentMember(top, "watches")
	req.policy, _ = presentMember(top, "policy")
	req.goal, _ = presentMember(top, "goal")
	if req.watches != nil {
		if _, err := parseWatchesMember(req.watches); isInvalidBody(err) {
			return setupRequest{}, err
		}
	}
	if req.policy != nil {
		if _, err := parsePolicyMember(req.policy); isInvalidBody(err) {
			return setupRequest{}, err
		}
	}
	if req.goal != nil {
		var obj map[string]json.RawMessage
		if json.Unmarshal(req.goal, &obj) != nil || obj == nil {
			return setupRequest{}, bodyErr("goal", "goal must be an object")
		}
	}
	return req, nil
}

func isInvalidBody(err error) bool {
	var se *SettingsError
	return errors.As(err, &se) && se.Code == codeInvalidBody
}

// validateSetup runs the ordered validation: identity, watches, goal,
// context, may-do. The first failure is returned. A part's reads happen when
// that part is reached.
func (s *Service) validateSetup(ctx context.Context, workspaceID string, req setupRequest) (*setupPlan, error) {
	plan := &setupPlan{agentProfileID: req.agentProfileID, executorProfileID: req.executorProfileID}
	var err error
	if plan.name, err = ValidateName(req.name); err != nil {
		return nil, stepError(setupStepIdentity, err)
	}
	if err = s.validator.ValidateAgentProfile(ctx, workspaceID, req.agentProfileID); err != nil {
		return nil, stepError(setupStepIdentity, err)
	}
	if err = s.validator.ValidateExecutorProfile(ctx, req.executorProfileID); err != nil {
		return nil, stepError(setupStepIdentity, err)
	}
	if plan.watches, err = s.validateSetupWatches(ctx, workspaceID, req.watches); err != nil {
		return nil, err
	}
	if plan.goal, err = validateSetupGoal(req.goal); err != nil {
		return nil, err
	}
	if plan.context, err = ValidateContext(req.context); err != nil {
		return nil, stepError(setupStepContext, err)
	}
	if plan.policy, err = validateSetupPolicy(req.policy); err != nil {
		return nil, err
	}
	return plan, nil
}

func (s *Service) validateSetupWatches(ctx context.Context, workspaceID string, raw json.RawMessage) (*watchesRequest, error) {
	if raw == nil {
		return nil, stepError(setupStepWatches, watchesErr(codeInvalidScope, "watches.scope must be all or selected"))
	}
	w, err := parseWatchesMember(raw)
	if err != nil {
		return nil, stepError(setupStepWatches, err)
	}
	if w.scope == watchScopeAll {
		return w, nil
	}
	if len(w.ids) == 0 {
		return nil, stepError(setupStepWatches, watchesErr(codeWatchesEmpty, "a selected set needs at least one workflow"))
	}
	existing, err := s.store.existingWorkflows(ctx, s.store.ro, workspaceID, w.ids)
	if err != nil {
		return nil, err
	}
	for _, id := range w.ids {
		if _, ok := existing[id]; !ok {
			return nil, stepError(setupStepWatches, watchesErr(codeWatchesForeignWorkflow, "workflow "+id+" is not in this workspace"))
		}
	}
	return w, nil
}

func validateSetupGoal(raw json.RawMessage) (*goalInput, error) {
	if raw == nil {
		return nil, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, bodyErr("goal", "goal must be an object")
	}
	in, err := validateGoalInput(obj, nil)
	if err != nil {
		var fe *FieldError
		if errors.As(err, &fe) {
			return nil, stepError(setupStepGoal, &FieldError{Field: "goal." + fe.Field, Message: fe.Message})
		}
		return nil, err
	}
	return &in, nil
}

func validateSetupPolicy(raw json.RawMessage) (Policy, error) {
	if raw == nil {
		return Policy{}, stepError(setupStepMayDo, policyFieldErr(codeActionMissing, string(AllActions[0]), "policy.actions must name every action"))
	}
	p, err := parsePolicyMember(raw)
	if err != nil {
		return Policy{}, stepError(setupStepMayDo, err)
	}
	return *p, nil
}

// CreateSetup creates a coordinator with its policy, Watches and optional goal
// in one transaction, or none of them. body is the raw request body, decoded
// only after the caller is authorized.
func (s *Service) CreateSetup(ctx context.Context, workspaceID string, body []byte) (*Coordinator, error) {
	if !s.phase2 {
		return nil, ErrNotFound
	}
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	req, err := decodeSetupRequest(body)
	if err != nil {
		return nil, err
	}
	plan, err := s.validateSetup(ctx, workspaceID, req)
	if err != nil {
		return nil, err
	}
	created, err := s.insertSetup(ctx, workspaceID, plan)
	if err != nil {
		return nil, fmt.Errorf("guided setup: %w", err)
	}
	s.logger.Info("coordinator created by guided setup",
		zap.String("workspace_id", workspaceID), zap.String("coordinator_id", created.ID))
	s.publishCoordinatorUpdated(ctx, workspaceID, created.ID)
	return created, nil
}

// insertSetup writes the coordinator row, its Watches and its goal in one
// transaction, in that order; the goal baseline counts the watch rows just
// written.
func (s *Service) insertSetup(ctx context.Context, workspaceID string, plan *setupPlan) (*Coordinator, error) {
	policyJSON, err := json.Marshal(plan.policy)
	if err != nil {
		return nil, fmt.Errorf("encode policy: %w", err)
	}
	policy := string(policyJSON)
	now := s.store.now().UTC()
	c := &Coordinator{
		ID: uuid.NewString(), WorkspaceID: workspaceID, Name: plan.name,
		AgentProfileID: plan.agentProfileID, ExecutorProfileID: plan.executorProfileID, Context: plan.context,
		CreatedAt: now, UpdatedAt: now, PolicyJSON: &policy, PolicyRevision: 1, WatchScope: plan.watches.scope,
	}
	tx, err := s.store.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin setup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.insertSetupRows(ctx, tx, c, plan, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit setup: %w", err)
	}
	return c, nil
}

func (s *Service) insertSetupRows(ctx context.Context, tx coordinatorExec, c *Coordinator, plan *setupPlan, now time.Time) error {
	if _, err := tx.ExecContext(ctx, s.store.db.Rebind(`
		INSERT INTO coordinators (`+insertCoordinatorColumns+`, policy_json, policy_revision, watch_scope)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		c.ID, c.WorkspaceID, c.Name, c.AgentProfileID, c.ExecutorProfileID, c.Context,
		nullableString(c.ConversationTaskID), c.ConfigRevision, now, now, *c.PolicyJSON, c.PolicyRevision, c.WatchScope); err != nil {
		return fmt.Errorf("insert coordinator: %w", err)
	}
	if plan.watches.scope == watchScopeSelected {
		for _, id := range plan.watches.ids {
			if _, err := tx.ExecContext(ctx, s.store.db.Rebind(`INSERT INTO coordinator_watches (coordinator_id, workflow_id, workspace_id, created_at) VALUES (?, ?, ?, ?)`),
				c.ID, id, c.WorkspaceID, now); err != nil {
				return fmt.Errorf("insert watch: %w", err)
			}
		}
	}
	if plan.goal == nil {
		return nil
	}
	_, err := s.createGoal(ctx, tx, c, *plan.goal)
	return err
}
