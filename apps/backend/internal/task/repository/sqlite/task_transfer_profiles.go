package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type transferParticipantProfile struct {
	ProfileID       string `db:"profile_id"`
	ParticipantRole string `db:"participant_role"`
	WorkspaceID     string `db:"workspace_id"`
	ProfileRole     string `db:"profile_role"`
	ProfileExists   int    `db:"profile_exists"`
}

func (r *Repository) validateTransferCoordinatorActor(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
) error {
	if command.Actor.Kind != models.TaskTransferActorCoordinator {
		return nil
	}
	var bindings int
	if err := tx.GetContext(ctx, &bindings, r.db.Rebind(`
		SELECT COUNT(*) FROM task_sessions session
		JOIN tasks caller ON caller.id = session.task_id
		JOIN workspaces source_workspace ON source_workspace.id = caller.workspace_id
		JOIN workflow_step_participants participant
			ON participant.task_id = caller.id AND participant.step_id = caller.workflow_step_id
			AND participant.role = 'runner' AND participant.agent_profile_id = session.agent_profile_id
		JOIN agent_profiles profile ON profile.id = session.agent_profile_id
		WHERE session.id = ? AND session.task_id = ? AND session.agent_profile_id = ? AND session.state = ?
			AND caller.workspace_id = ?
			AND `+IsFromOfficePredicate("caller")+`
			AND profile.workspace_id = ? AND profile.role = 'ceo' AND profile.deleted_at IS NULL
			AND profile.status IN ('idle', 'working')`),
		command.Actor.SessionID, command.Actor.CallerTaskID, command.Actor.ID,
		models.TaskSessionStateRunning, command.ExpectedSourceWorkspaceID, command.ExpectedSourceWorkspaceID); err != nil {
		return err
	}
	if bindings != 1 {
		return fmt.Errorf("%w: coordinator session binding changed", repoerrors.ErrTaskTransferConflict)
	}
	return nil
}

func (r *Repository) resolveTransferAgentProfileMappings(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	inventory transferRelationInventory,
) (map[string]string, error) {
	if !inventory.agentProfiles {
		return r.rejectUnverifiableTransferProfiles(ctx, tx, command)
	}
	if err := r.validateTransferEffectiveProfile(ctx, tx, command.ExpectedSourceStepID); err != nil {
		return nil, err
	}
	participants, err := r.taskTransferParticipantProfiles(ctx, tx, command.TaskID)
	if err != nil {
		return nil, err
	}
	return r.mapTaskTransferParticipantProfiles(ctx, tx, command, participants)
}

func (r *Repository) rejectUnverifiableTransferProfiles(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
) (map[string]string, error) {
	var profileCount int
	if err := tx.GetContext(ctx, &profileCount, r.db.Rebind(`
		SELECT (SELECT COUNT(*) FROM workflow_step_participants WHERE task_id = ? AND COALESCE(agent_profile_id, '') <> '') +
			(SELECT COUNT(*) FROM workflow_steps s JOIN workflows w ON w.id = s.workflow_id
			 WHERE s.id = ? AND COALESCE(NULLIF(s.agent_profile_id, ''), w.agent_profile_id, '') <> '')`),
		command.TaskID, command.ExpectedSourceStepID); err != nil {
		return nil, err
	}
	if profileCount != 0 {
		return nil, fmt.Errorf("%w: destination agent profile mapping is required", repoerrors.ErrTaskTransferConflict)
	}
	return map[string]string{}, nil
}

func (r *Repository) validateTransferEffectiveProfile(
	ctx context.Context,
	tx *sqlx.Tx,
	stepID string,
) error {
	var profileID string
	if err := tx.GetContext(ctx, &profileID, r.db.Rebind(`
		SELECT COALESCE(NULLIF(s.agent_profile_id, ''), w.agent_profile_id, '')
		FROM workflow_steps s JOIN workflows w ON w.id = s.workflow_id WHERE s.id = ?`), stepID); err != nil {
		return err
	}
	if profileID == "" {
		return nil
	}
	var workspaceID string
	if err := tx.GetContext(ctx, &workspaceID, r.db.Rebind(
		`SELECT COALESCE(workspace_id, '') FROM agent_profiles WHERE id = ?`), profileID); err != nil {
		return taskTransferConflict(err, "effective agent profile unavailable")
	}
	if workspaceID != "" {
		return fmt.Errorf("%w: destination agent profile mapping is required", repoerrors.ErrTaskTransferConflict)
	}
	return nil
}

func (r *Repository) taskTransferParticipantProfiles(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) ([]transferParticipantProfile, error) {
	var participants []transferParticipantProfile
	err := tx.SelectContext(ctx, &participants, r.db.Rebind(`
		SELECT p.agent_profile_id AS profile_id, p.role AS participant_role,
			COALESCE(a.workspace_id, '') AS workspace_id, COALESCE(a.role, '') AS profile_role,
			CASE WHEN a.id IS NULL THEN 0 ELSE 1 END AS profile_exists
		FROM workflow_step_participants p
		LEFT JOIN agent_profiles a ON a.id = p.agent_profile_id
		WHERE p.task_id = ? AND p.agent_profile_id <> ''`), taskID)
	return participants, err
}

func (r *Repository) mapTaskTransferParticipantProfiles(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	participants []transferParticipantProfile,
) (map[string]string, error) {
	mappings := make(map[string]string)
	for _, participant := range participants {
		if participant.ProfileExists == 0 {
			return nil, fmt.Errorf("%w: task participant agent profile is unavailable", repoerrors.ErrTaskTransferConflict)
		}
		if participant.WorkspaceID == "" {
			continue
		}
		if !canMapTransferCEOProfile(command, participant) {
			return nil, fmt.Errorf("%w: destination agent profile mapping is required", repoerrors.ErrTaskTransferConflict)
		}
		destinationProfileID, err := r.destinationTransferCEOProfile(ctx, tx, command.DestinationWorkspaceID)
		if err != nil {
			return nil, err
		}
		mappings[participant.ProfileID] = destinationProfileID
	}
	return mappings, nil
}

func canMapTransferCEOProfile(command models.TaskTransferCommand, participant transferParticipantProfile) bool {
	if participant.ParticipantRole != "runner" || participant.WorkspaceID != command.ExpectedSourceWorkspaceID ||
		participant.ProfileRole != "ceo" {
		return false
	}
	return command.Actor.Kind == models.TaskTransferActorHuman ||
		(command.Actor.Kind == models.TaskTransferActorCoordinator && participant.ProfileID == command.Actor.ID)
}

func (r *Repository) destinationTransferCEOProfile(
	ctx context.Context,
	tx *sqlx.Tx,
	workspaceID string,
) (string, error) {
	var candidates []string
	if err := tx.SelectContext(ctx, &candidates, r.db.Rebind(`
		SELECT id FROM agent_profiles
		WHERE workspace_id = ? AND role = 'ceo' AND deleted_at IS NULL AND enabled = 1
			AND status IN ('idle', 'working') ORDER BY id`), workspaceID); err != nil {
		return "", err
	}
	if len(candidates) != 1 {
		return "", fmt.Errorf("%w: destination CEO profile mapping is not unique", repoerrors.ErrTaskTransferConflict)
	}
	return candidates[0], nil
}
