package coordinator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/authz"
	"go.uber.org/zap"
)

// maxWatchedWorkflows bounds a selected Watches set.
const maxWatchedWorkflows = 50

// Settings error codes: the closed set of the settings PUT's 400 answers.
const (
	codeInvalidBody            = "invalid_body"
	codeUnknownAction          = "unknown_action"
	codeActionMissing          = "action_missing"
	codeInvalidSetting         = "invalid_setting"
	codeStopDeniedOnly         = "stop_denied_only"
	codeAutomaticNotAvailable  = "automatic_not_available"
	codeInvalidScope           = "invalid_scope"
	codeWatchesEmpty           = "watches_empty"
	codeWatchesTooMany         = "watches_too_many"
	codeWatchesDuplicate       = "watches_duplicate"
	codeWatchesForeignWorkflow = "watches_foreign_workflow"
)

// SettingsError is a 400 the settings save answers with: a closed Code, and
// the Field it is about.
type SettingsError struct {
	Code    string
	Field   string
	Message string
	// Step names the guided-setup step that owns the value; empty elsewhere.
	Step string
}

func (e *SettingsError) Error() string { return e.Message }

func policyFieldErr(code, action, message string) *SettingsError {
	return &SettingsError{Code: code, Field: "policy.actions." + action, Message: message}
}

func watchesErr(code, message string) *SettingsError {
	return &SettingsError{Code: code, Field: "watches", Message: message}
}

func bodyErr(field, message string) *SettingsError {
	return &SettingsError{Code: codeInvalidBody, Field: field, Message: message}
}

// PolicyDeniedError is the approve re-check refusal: the named action is
// denied by the coordinator's stored policy.
type PolicyDeniedError struct {
	Action Action
}

func (e *PolicyDeniedError) Error() string { return "policy_denied" }

// settingsRequest is a validated PUT body. Nil members are unchanged.
type settingsRequest struct {
	policy   *Policy
	watches  *watchesRequest
	projects *projectsRequest
}

type watchesRequest struct {
	scope string
	ids   []string
}

// parseSettingsBody validates everything in the body that needs no stored
// state. Policy is validated before Watches.
func parseSettingsBody(body []byte, phase3, phase31 bool) (settingsRequest, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil || top == nil {
		return settingsRequest{}, bodyErr("", "body must be a JSON object")
	}
	var req settingsRequest
	if raw, ok := presentMember(top, "policy"); ok {
		p, err := parsePolicyMember(raw, phase3)
		if err != nil {
			return settingsRequest{}, err
		}
		req.policy = p
	}
	if raw, ok := presentMember(top, "watches"); ok {
		w, err := parseWatchesMember(raw)
		if err != nil {
			return settingsRequest{}, err
		}
		req.watches = w
	}
	if raw, ok := presentMember(top, fieldProjects); ok && phase31 {
		p, err := parseProjectsMember(raw)
		if err != nil {
			return settingsRequest{}, err
		}
		req.projects = p
	}
	return req, nil
}

// presentMember returns a member that is present and not JSON null.
func presentMember(obj map[string]json.RawMessage, key string) (json.RawMessage, bool) {
	raw, ok := obj[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, false
	}
	return raw, true
}

func parsePolicyMember(raw json.RawMessage, phase3 bool) (*Policy, error) {
	var member map[string]json.RawMessage
	if err := json.Unmarshal(raw, &member); err != nil || member == nil {
		return nil, bodyErr("policy", "policy must be an object")
	}
	rawActions, ok := presentMember(member, "actions")
	if !ok {
		return nil, policyFieldErr(codeActionMissing, string(AllActions[0]), "policy.actions must name every action")
	}
	var actions map[string]json.RawMessage
	if err := json.Unmarshal(rawActions, &actions); err != nil || actions == nil {
		return nil, bodyErr("policy.actions", "policy.actions must be an object")
	}
	var unknown []string
	for key := range actions {
		if !isPolicyAction(Action(key)) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, policyFieldErr(codeUnknownAction, unknown[0], "unknown action "+unknown[0])
	}
	for _, a := range AllActions {
		if _, ok := actions[string(a)]; !ok {
			return nil, policyFieldErr(codeActionMissing, string(a), "policy.actions must name every action")
		}
	}
	p := Policy{Version: policyVersion, Actions: make(map[Action]Setting, len(AllActions))}
	for _, a := range AllActions {
		var s string
		if err := json.Unmarshal(actions[string(a)], &s); err != nil {
			s = ""
		}
		p.Actions[a] = Setting(s)
	}
	if err := Validate(p, phase3); err != nil {
		return nil, policyValidateErr(err, p)
	}
	return &p, nil
}

func policyValidateErr(err error, p Policy) *SettingsError {
	fe, ok := err.(*PolicyFieldError)
	if !ok {
		return bodyErr("policy", err.Error())
	}
	code := codeInvalidSetting
	switch {
	case !validSetting(p.Actions[Action(fe.Field)]):
		return policyFieldErr(codeInvalidSetting, fe.Field, "invalid setting for "+fe.Field)
	case fe.Code == codeAutomaticNotAvailable:
		code = codeAutomaticNotAvailable
	case Action(fe.Field) == ActionStop && p.Actions[ActionStop] != SettingDenied:
		code = codeStopDeniedOnly
	}
	return policyFieldErr(code, fe.Field, fe.Error())
}

func parseWatchesMember(raw json.RawMessage) (*watchesRequest, error) {
	var member map[string]json.RawMessage
	if err := json.Unmarshal(raw, &member); err != nil || member == nil {
		return nil, bodyErr("watches", "watches must be an object")
	}
	scopeRaw, ok := presentMember(member, "scope")
	if !ok {
		return nil, watchesErr(codeInvalidScope, "watches.scope must be all or selected")
	}
	var scope string
	if err := json.Unmarshal(scopeRaw, &scope); err != nil {
		return nil, bodyErr("watches", "watches.scope must be a string")
	}
	if scope != watchScopeAll && scope != watchScopeSelected {
		return nil, watchesErr(codeInvalidScope, "watches.scope must be all or selected")
	}
	w := &watchesRequest{scope: scope}
	if scope == watchScopeAll {
		return w, nil
	}
	idsRaw, ok := presentMember(member, "workflow_ids")
	if !ok {
		return w, nil
	}
	if err := json.Unmarshal(idsRaw, &w.ids); err != nil {
		return nil, bodyErr("watches", "watches.workflow_ids must be an array of strings")
	}
	if len(w.ids) > maxWatchedWorkflows {
		return nil, watchesErr(codeWatchesTooMany, fmt.Sprintf("at most %d workflows may be watched", maxWatchedWorkflows))
	}
	seen := make(map[string]struct{}, len(w.ids))
	for _, id := range w.ids {
		if _, dup := seen[id]; dup {
			return nil, watchesErr(codeWatchesDuplicate, "duplicate workflow id "+id)
		}
		seen[id] = struct{}{}
	}
	return w, nil
}

// watchState is a coordinator's scope and effective selected workflow ids.
type watchState struct {
	scope string
	ids   []string
}

func (w watchState) equal(o watchState) bool {
	return w.scope == o.scope && (w.scope == watchScopeAll || slices.Equal(w.ids, o.ids))
}

// existingWorkflows returns the subset of ids naming a workflow of the
// workspace, hidden workflows included, in one query.
func (s *Store) existingWorkflows(ctx context.Context, exec coordinatorExec, workspaceID string, ids []string) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, workspaceID)
	for _, id := range ids {
		args = append(args, id)
	}
	query := `SELECT id FROM workflows WHERE workspace_id = ? AND id IN (?` + strings.Repeat(",?", len(ids)-1) + `)`
	rows, err := exec.QueryContext(ctx, s.db.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("read workflows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan workflow: %w", err)
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// EffectiveWatchSet is the coordinator's Watches with every stored id whose
// workflow no longer exists in the workspace omitted.
func (s *Store) EffectiveWatchSet(ctx context.Context, exec coordinatorExec, coordinatorID, workspaceID string) (WatchSet, error) {
	set, err := s.LoadWatchSet(ctx, exec, coordinatorID)
	if err != nil || set.All {
		return set, err
	}
	existing, err := s.existingWorkflows(ctx, exec, workspaceID, set.WorkflowIDs)
	if err != nil {
		return WatchSet{}, err
	}
	ids := make([]string, 0, len(set.WorkflowIDs))
	for _, id := range set.WorkflowIDs {
		if _, ok := existing[id]; ok {
			ids = append(ids, id)
		}
	}
	set.All = false
	set.WorkflowIDs = ids
	return set, nil
}

// EffectiveWatchSet returns the coordinator's effective Watches, read through
// the reader pool.
func (s *Service) EffectiveWatchSet(ctx context.Context, coordinatorID string) (WatchSet, error) {
	c, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return WatchSet{}, err
	}
	return s.store.EffectiveWatchSet(ctx, s.store.ro, c.ID, c.WorkspaceID)
}

// GetSettings returns the coordinator's policy, revision and effective Watches.
func (s *Service) GetSettings(ctx context.Context, workspaceID, coordinatorID string) (*CoordinatorPhase2, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	if _, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID); err != nil {
		return nil, err
	}
	view, err := s.Policy(ctx, coordinatorID)
	if err != nil {
		return nil, err
	}
	out := phase2FromView(view)
	state, err := s.store.loadProjectState(ctx, s.store.ro, coordinatorID)
	if err != nil {
		return nil, err
	}
	out.Projects, out.ProjectsConfig = s.projectsView(ctx, workspaceID, state)
	return out, nil
}

func phase2FromView(v PolicyView) *CoordinatorPhase2 {
	ids := v.WorkflowIDs
	if ids == nil {
		ids = []string{}
	}
	return &CoordinatorPhase2{
		Policy:         CoordinatorPolicyDTO{Actions: v.Actions},
		PolicyRevision: v.PolicyRevision,
		Watches:        CoordinatorWatchDTO{Scope: v.WatchScope, WorkflowIDs: ids},
	}
}

// SaveSettings applies a settings PUT body and returns the stored result.
func (s *Service) SaveSettings(ctx context.Context, workspaceID, coordinatorID string, body []byte) (*CoordinatorPhase2, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	if _, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID); err != nil {
		return nil, err
	}
	req, err := parseSettingsBody(body, s.phase3, s.phase31)
	if err != nil {
		return nil, err
	}
	var (
		result   *CoordinatorPhase2
		archived string
		changed  bool
	)
	err = s.store.withCoordinatorLock(ctx, coordinatorID, func(tx coordinatorExec) error {
		var err error
		result, changed, err = s.applySettings(ctx, tx, workspaceID, coordinatorID, req)
		if err != nil || !changed {
			return err
		}
		archived, err = s.resetConversation(ctx, tx, coordinatorID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if changed {
		s.archiveConversation(ctx, coordinatorID, archived)
		s.publishCoordinatorUpdated(ctx, workspaceID, coordinatorID)
	}
	return result, nil
}

// applySettings runs inside the coordinator lock: it reads the stored state,
// resolves the body against the effective Watches, and writes only when
// something differs.
func (s *Service) applySettings(ctx context.Context, tx coordinatorExec, workspaceID, coordinatorID string, req settingsRequest) (*CoordinatorPhase2, bool, error) {
	row, err := s.readSettingsRow(ctx, tx, workspaceID, coordinatorID)
	if err != nil {
		return nil, false, err
	}
	stored, policyErr := ParsePolicy(row.PolicyJSON)
	rawSet, err := s.store.LoadWatchSet(ctx, tx, coordinatorID)
	if err != nil {
		return nil, false, err
	}
	current, err := s.effectiveWatchState(ctx, tx, workspaceID, rawSet)
	if err != nil {
		return nil, false, err
	}
	newWatches, err := s.resolveWatches(ctx, tx, workspaceID, rawSet, current, req.watches)
	if err != nil {
		return nil, false, err
	}
	storedProjects, newProjects, err := s.resolveProjectsFor(ctx, tx, workspaceID, coordinatorID, req.projects)
	if err != nil {
		return nil, false, err
	}
	policyChanged := req.policy != nil && (policyErr != nil || !policiesEqual(*req.policy, stored))
	watchesChanged := newWatches != nil
	final := stored
	if policyChanged {
		final = *req.policy
	}
	scope := current
	if watchesChanged {
		scope = *newWatches
	}
	if newProjects != nil {
		storedProjects = *newProjects
	}
	result := &CoordinatorPhase2{
		Policy:         CoordinatorPolicyDTO{Actions: final.Actions},
		PolicyRevision: row.Revision,
		Watches:        CoordinatorWatchDTO{Scope: scope.scope, WorkflowIDs: nonNilIDs(scope.ids)},
	}
	result.Projects, result.ProjectsConfig = s.projectsView(ctx, workspaceID, storedProjects)
	if !policyChanged && !watchesChanged && newProjects == nil {
		return result, false, nil
	}
	if err := s.commitSettings(ctx, tx, workspaceID, coordinatorID, row.PolicyJSON, stored, final, policyChanged, watchesChanged, scope, newProjects); err != nil {
		return nil, false, err
	}
	result.PolicyRevision = row.Revision + 1
	s.logger.Info("coordinator settings saved",
		zap.String("coordinator_id", coordinatorID), zap.Int("old_revision", row.Revision),
		zap.Int("new_revision", row.Revision+1), zap.Bool("policy_changed", policyChanged), zap.Bool("watches_changed", watchesChanged), zap.Bool("projects_changed", newProjects != nil))
	return result, true, nil
}

type settingsRow struct {
	PolicyJSON *string
	Revision   int
}

// readSettingsRow reads the stored policy and revision; a coordinator of
// another workspace is not found.
func (s *Service) readSettingsRow(ctx context.Context, tx coordinatorExec, workspaceID, coordinatorID string) (settingsRow, error) {
	var row settingsRow
	var owner string
	if err := tx.QueryRowContext(ctx, s.store.db.Rebind(`SELECT policy_json, policy_revision, workspace_id FROM coordinators WHERE id = ?`), coordinatorID).
		Scan(&row.PolicyJSON, &row.Revision, &owner); err != nil {
		return settingsRow{}, fmt.Errorf("read coordinator settings: %w", err)
	}
	if owner != workspaceID {
		return settingsRow{}, ErrNotFound
	}
	return row, nil
}

// resolveProjectsFor loads the stored project state and resolves the request
// against it; the second result is nil when nothing changes.
func (s *Service) resolveProjectsFor(ctx context.Context, tx coordinatorExec, workspaceID, coordinatorID string, req *projectsRequest) (projectState, *projectState, error) {
	stored, err := s.store.loadProjectState(ctx, tx, coordinatorID)
	if err != nil {
		return projectState{}, nil, err
	}
	next, err := s.resolveProjects(ctx, workspaceID, stored, req)
	return stored, next, err
}

// commitSettings gates a raise, writes the changed settings and records the
// class changes, all inside the caller's coordinator lock.
func (s *Service) commitSettings(ctx context.Context, tx coordinatorExec, workspaceID, coordinatorID string, storedJSON *string, stored, final Policy, policyChanged, watchesChanged bool, scope watchState, projects *projectState) error {
	if policyChanged {
		if err := s.checkRaise(ctx, coordinatorID, stored, final); err != nil {
			return err
		}
	}
	if err := s.writeSettings(ctx, tx, workspaceID, coordinatorID, storedJSON, policyChanged, final, watchesChanged, scope, projects); err != nil {
		return err
	}
	if policyChanged {
		return s.recordClassChanges(ctx, tx, coordinatorID, stored, final)
	}
	return nil
}

func nonNilIDs(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

func policiesEqual(a, b Policy) bool {
	for _, action := range AllActions {
		if a.Actions[action] != b.Actions[action] {
			return false
		}
	}
	return true
}

func (s *Service) effectiveWatchState(ctx context.Context, tx coordinatorExec, workspaceID string, raw WatchSet) (watchState, error) {
	if raw.All {
		return watchState{scope: watchScopeAll}, nil
	}
	existing, err := s.store.existingWorkflows(ctx, tx, workspaceID, raw.WorkflowIDs)
	if err != nil {
		return watchState{}, err
	}
	ids := []string{}
	for _, id := range raw.WorkflowIDs {
		if _, ok := existing[id]; ok {
			ids = append(ids, id)
		}
	}
	return watchState{scope: watchScopeSelected, ids: ids}, nil
}

// resolveWatches turns the body's Watches member into the state to write, or
// nil when it leaves the effective Watches as they are. A body id that is
// stored but whose workflow is gone is dropped; any other id that is not a
// workflow of the workspace is foreign; a changed selected set may not be empty.
func (s *Service) resolveWatches(ctx context.Context, tx coordinatorExec, workspaceID string, raw WatchSet, current watchState, w *watchesRequest) (*watchState, error) {
	if w == nil {
		return nil, nil
	}
	if w.scope == watchScopeAll {
		next := watchState{scope: watchScopeAll}
		if next.equal(current) {
			return nil, nil
		}
		return &next, nil
	}
	existing, err := s.store.existingWorkflows(ctx, tx, workspaceID, w.ids)
	if err != nil {
		return nil, err
	}
	stored := make(map[string]struct{}, len(raw.WorkflowIDs))
	for _, id := range raw.WorkflowIDs {
		stored[id] = struct{}{}
	}
	next := watchState{scope: watchScopeSelected, ids: []string{}}
	for _, id := range w.ids {
		if _, ok := existing[id]; ok {
			next.ids = append(next.ids, id)
			continue
		}
		if _, ok := stored[id]; !ok {
			return nil, watchesErr(codeWatchesForeignWorkflow, "workflow "+id+" is not in this workspace")
		}
	}
	sort.Strings(next.ids)
	if next.equal(current) {
		return nil, nil
	}
	if len(next.ids) == 0 {
		return nil, watchesErr(codeWatchesEmpty, "a selected set needs at least one workflow")
	}
	return &next, nil
}

func (s *Service) writeSettings(ctx context.Context, tx coordinatorExec, workspaceID, coordinatorID string, oldPolicy *string, policyChanged bool, final Policy, watchesChanged bool, scope watchState, projects *projectState) error {
	policyJSON := oldPolicy
	if policyChanged {
		encoded, err := json.Marshal(final)
		if err != nil {
			return fmt.Errorf("encode policy: %w", err)
		}
		v := string(encoded)
		policyJSON = &v
	}
	query := `UPDATE coordinators SET policy_json = ?, policy_revision = policy_revision + 1, updated_at = ?`
	args := []any{policyJSON, s.store.now().UTC()}
	if watchesChanged {
		query += `, watch_scope = ?`
		args = append(args, scope.scope)
	}
	if _, err := tx.ExecContext(ctx, s.store.db.Rebind(query+` WHERE id = ?`), append(args, coordinatorID)...); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	if projects != nil {
		if err := s.store.writeProjectState(ctx, tx, workspaceID, coordinatorID, *projects); err != nil {
			return err
		}
	}
	if !watchesChanged {
		return nil
	}
	if _, err := tx.ExecContext(ctx, s.store.db.Rebind(`DELETE FROM coordinator_watches WHERE coordinator_id = ?`), coordinatorID); err != nil {
		return fmt.Errorf("clear watches: %w", err)
	}
	for _, id := range scope.ids {
		if _, err := tx.ExecContext(ctx, s.store.db.Rebind(`INSERT INTO coordinator_watches (coordinator_id, workflow_id, workspace_id, created_at) VALUES (?, ?, ?, ?)`),
			coordinatorID, id, workspaceID, s.store.now().UTC()); err != nil {
			return fmt.Errorf("insert watch: %w", err)
		}
	}
	return nil
}
