package automation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type managedScheduleOperation struct {
	PayloadDigest    string `db:"payload_digest"`
	AutomationID     string `db:"automation_id"`
	ResourceRevision uint64 `db:"resource_revision"`
}

func (s *Store) ManagedScheduleOperationApplied(ctx context.Context, operationID, payloadDigest, automationID string) (bool, error) {
	var operation managedScheduleOperation
	err := s.ro.GetContext(ctx, &operation, s.ro.Rebind(`SELECT payload_digest, automation_id, resource_revision
		FROM automation_managed_schedule_operations WHERE operation_id=?`), operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if operation.PayloadDigest != payloadDigest || operation.AutomationID != automationID {
		return false, ErrManagedScheduleIdempotencyConflict
	}
	return true, nil
}

func (s *Store) ListManagedSchedules(ctx context.Context, installationID, workspaceID string) ([]*Automation, error) {
	var items []*Automation
	err := s.ro.SelectContext(ctx, &items, s.ro.Rebind(`SELECT `+automationColumns+` FROM automations
		WHERE managed_owner_installation_id = ? AND workspace_id = ? AND task_mode = ? ORDER BY name, id`),
		installationID, workspaceID, TaskModeManagedConversation)
	if err != nil {
		return nil, err
	}
	return s.hydrateManagedSchedules(ctx, items)
}

func (s *Store) GetOwnedManagedSchedule(ctx context.Context, installationID, workspaceID, id string) (*Automation, error) {
	var item Automation
	err := s.ro.GetContext(ctx, &item, s.ro.Rebind(`SELECT `+automationColumns+` FROM automations
		WHERE id = ? AND managed_owner_installation_id = ? AND workspace_id = ? AND task_mode = ?`),
		id, installationID, workspaceID, TaskModeManagedConversation)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrManagedScheduleNotFound
	}
	if err != nil {
		return nil, err
	}
	items, err := s.hydrateManagedSchedules(ctx, []*Automation{&item})
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

func (s *Store) hydrateManagedSchedules(ctx context.Context, items []*Automation) ([]*Automation, error) {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		hydrateManagedDestination(item)
		ids = append(ids, item.ID)
	}
	triggers, err := s.listTriggersForAutomations(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("hydrate managed schedule triggers: %w", err)
	}
	for _, item := range items {
		item.Triggers = triggers[item.ID]
	}
	return items, nil
}

func (s *Store) CreateManagedSchedule(ctx context.Context, item *Automation, triggers []CreateTriggerSpec, operationID, payloadDigest string) (bool, error) {
	if item == nil || item.ID == "" || item.ManagedOwnerInstallationID == "" || operationID == "" || payloadDigest == "" {
		return false, ErrManagedScheduleInvalid
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if replayed, err := managedScheduleOperationReplay(ctx, tx, operationID, payloadDigest, item.ID); err != nil || replayed {
		return replayed, err
	}
	item.CreatedAt = time.Now().UTC()
	item.UpdatedAt = item.CreatedAt
	if item.ResourceRevision == 0 {
		item.ResourceRevision = 1
	}
	if item.WebhookSecret == "" {
		item.WebhookSecret = generateSecret()
	}
	setManagedDestinationColumns(item)
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO automations (id, workspace_id, name, description,
		workflow_id, workflow_step_id, agent_profile_id, executor_profile_id, task_mode,
		managed_owner_installation_id, managed_destination_installation_id, managed_destination_conversation_id,
		managed_destination_plugin_id, managed_destination_instance_key, managed_destination_revision,
		resource_revision, repository_mode, prompt, task_title_template, execution_mode, enabled,
		max_concurrent_runs, continuation_policy, continuation_task_id, webhook_secret, last_triggered_at,
		created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?)`),
		item.ID, item.WorkspaceID, item.Name, item.Description, item.WorkflowID, item.WorkflowStepID,
		item.AgentProfileID, item.ExecutorProfileID, item.TaskMode, item.ManagedOwnerInstallationID,
		item.ManagedDestinationInstallationID, item.ManagedDestinationConversationID,
		item.ManagedDestinationPluginID, item.ManagedDestinationInstanceKey, item.ManagedDestinationRevision,
		item.ResourceRevision, item.RepositoryMode, item.Prompt, item.TaskTitleTemplate, item.Enabled,
		item.MaxConcurrentRuns, item.ContinuationPolicy, item.ContinuationTaskID, item.WebhookSecret,
		item.LastTriggeredAt, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return false, err
	}
	for _, spec := range triggers {
		if err := insertManagedScheduleTrigger(ctx, tx, item.ID, spec); err != nil {
			return false, err
		}
	}
	if err := saveManagedScheduleOperation(ctx, tx, operationID, payloadDigest, item.ID, "create", item.ResourceRevision); err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func (s *Store) UpdateManagedSchedule(ctx context.Context, installationID, workspaceID, id string, expectedRevision uint64, item *Automation, triggers []CreateTriggerSpec, operationID, payloadDigest string) (bool, uint64, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if replayed, err := managedScheduleOperationReplay(ctx, tx, operationID, payloadDigest, id); err != nil || replayed {
		return replayed, 0, err
	}
	current, err := ownedManagedScheduleRevision(ctx, tx, installationID, workspaceID, id)
	if err != nil {
		return false, 0, err
	}
	if current != expectedRevision {
		return false, current, ErrManagedScheduleRevisionConflict
	}
	nextRevision := current + 1
	updated := time.Now().UTC()
	setManagedDestinationColumns(item)
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automations SET name=?, description=?, prompt=?,
		managed_destination_installation_id=?, managed_destination_conversation_id=?,
		managed_destination_plugin_id=?, managed_destination_instance_key=?, managed_destination_revision=?,
		resource_revision=?, updated_at=? WHERE id=? AND workspace_id=? AND managed_owner_installation_id=?
		AND task_mode=? AND resource_revision=?`),
		item.Name, item.Description, item.Prompt, item.ManagedDestinationInstallationID,
		item.ManagedDestinationConversationID, item.ManagedDestinationPluginID, item.ManagedDestinationInstanceKey,
		item.ManagedDestinationRevision, nextRevision, updated, id, workspaceID, installationID,
		TaskModeManagedConversation, expectedRevision)
	if err != nil {
		return false, 0, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return false, 0, ErrManagedScheduleRevisionConflict
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM automation_triggers WHERE automation_id = ?`), id); err != nil {
		return false, 0, err
	}
	for _, spec := range triggers {
		if err := insertManagedScheduleTrigger(ctx, tx, id, spec); err != nil {
			return false, 0, err
		}
	}
	if err := saveManagedScheduleOperation(ctx, tx, operationID, payloadDigest, id, "update", nextRevision); err != nil {
		return false, 0, err
	}
	return false, nextRevision, tx.Commit()
}

func (s *Store) SetManagedScheduleEnabled(ctx context.Context, installationID, workspaceID, id string, expectedRevision uint64, enabled bool, operationID, payloadDigest string) (bool, uint64, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if replayed, err := managedScheduleOperationReplay(ctx, tx, operationID, payloadDigest, id); err != nil || replayed {
		return replayed, 0, err
	}
	current, err := ownedManagedScheduleRevision(ctx, tx, installationID, workspaceID, id)
	if err != nil {
		return false, 0, err
	}
	if current != expectedRevision {
		return false, current, ErrManagedScheduleRevisionConflict
	}
	nextRevision := current + 1
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automations SET enabled=?, resource_revision=?, updated_at=?
		WHERE id=? AND workspace_id=? AND managed_owner_installation_id=? AND task_mode=? AND resource_revision=?`),
		enabled, nextRevision, time.Now().UTC(), id, workspaceID, installationID, TaskModeManagedConversation, expectedRevision)
	if err != nil {
		return false, 0, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return false, 0, ErrManagedScheduleRevisionConflict
	}
	if err := saveManagedScheduleOperation(ctx, tx, operationID, payloadDigest, id, "set_enabled", nextRevision); err != nil {
		return false, 0, err
	}
	return false, nextRevision, tx.Commit()
}

func (s *Store) DeleteManagedSchedule(ctx context.Context, installationID, workspaceID, id string, expectedRevision uint64, operationID, payloadDigest string) (bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if replayed, err := managedScheduleOperationReplay(ctx, tx, operationID, payloadDigest, id); err != nil || replayed {
		return replayed, err
	}
	current, err := ownedManagedScheduleRevision(ctx, tx, installationID, workspaceID, id)
	if err != nil {
		return false, err
	}
	if current != expectedRevision {
		return false, ErrManagedScheduleRevisionConflict
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM automations WHERE id=? AND workspace_id=?
		AND managed_owner_installation_id=? AND task_mode=? AND resource_revision=?`), id, workspaceID,
		installationID, TaskModeManagedConversation, expectedRevision)
	if err != nil {
		return false, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return false, ErrManagedScheduleRevisionConflict
	}
	if err := saveManagedScheduleOperation(ctx, tx, operationID, payloadDigest, id, "delete", current); err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func ownedManagedScheduleRevision(ctx context.Context, tx *sqlx.Tx, installationID, workspaceID, id string) (uint64, error) {
	var revision uint64
	err := tx.GetContext(ctx, &revision, tx.Rebind(`SELECT resource_revision FROM automations WHERE id=?
		AND workspace_id=? AND managed_owner_installation_id=? AND task_mode=?`),
		id, workspaceID, installationID, TaskModeManagedConversation)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrManagedScheduleNotFound
	}
	return revision, err
}

func managedScheduleOperationReplay(ctx context.Context, tx *sqlx.Tx, operationID, digest, id string) (bool, error) {
	var operation managedScheduleOperation
	err := tx.GetContext(ctx, &operation, tx.Rebind(`SELECT payload_digest, automation_id, resource_revision
		FROM automation_managed_schedule_operations WHERE operation_id=?`), operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if operation.PayloadDigest != digest || operation.AutomationID != id {
		return false, ErrManagedScheduleIdempotencyConflict
	}
	return true, nil
}

func saveManagedScheduleOperation(ctx context.Context, tx *sqlx.Tx, operationID, digest, id, operation string, revision uint64) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO automation_managed_schedule_operations
		(operation_id, payload_digest, automation_id, operation, resource_revision, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`), operationID, digest, id, operation, revision, time.Now().UTC())
	return err
}

func insertManagedScheduleTrigger(ctx context.Context, tx *sqlx.Tx, automationID string, spec CreateTriggerSpec) error {
	now := time.Now().UTC()
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO automation_triggers
		(id, automation_id, type, config, enabled, last_evaluated_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, NULL, ?, ?)`), uuid.NewString(), automationID, spec.Type, string(spec.Config), spec.Enabled, now, now)
	return err
}

func (s *Store) UpdateManagedRunDelivery(ctx context.Context, runID, inputID string, delivery ManagedDeliveryStatus, attempts int, errorMessage string) error {
	if runID == "" || attempts < 0 {
		return ErrManagedScheduleInvalid
	}
	const terminalAttempts = 3
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_runs SET
		managed_input_id = CASE WHEN ? = '' THEN managed_input_id ELSE ? END,
		delivery_status = ?, managed_delivery_attempts = ?, error_message = ?,
		status = CASE
			WHEN ? = ? THEN ?
			WHEN ? = ? OR (? = ? AND ? >= ?) THEN ?
			ELSE status
		END
		WHERE id = ? AND status IN (?, ?)`),
		inputID, inputID, delivery, attempts, errorMessage,
		string(delivery), string(ManagedDeliveryCompleted), string(RunStatusSucceeded),
		string(delivery), string(ManagedDeliveryFailed),
		string(delivery), string(ManagedDeliveryUnavailable), attempts, terminalAttempts, string(RunStatusFailed),
		runID, string(RunStatusTriggered), string(RunStatusTaskCreated))
	return err
}
