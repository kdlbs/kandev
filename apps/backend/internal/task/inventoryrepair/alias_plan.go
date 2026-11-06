package inventoryrepair

import "errors"

// AliasSelection identifies rows explicitly. Neither order nor age selects the owner.
type AliasSelection struct {
	WorktreeID     string   `json:"worktree_id"`
	CanonicalRowID string   `json:"canonical_row_id"`
	AliasRowIDs    []string `json:"alias_row_ids"`
}

type EnvironmentRebind struct {
	EnvironmentID          string   `json:"environment_id"`
	CanonicalEnvironmentID string   `json:"canonical_environment_id"`
	GroupID                string   `json:"group_id"`
	SessionIDs             []string `json:"session_ids"`
	SnapshotIDs            []string `json:"snapshot_ids"`
}

func (p Plan) validateAliases() error {
	if p.Version == 1 {
		if len(p.Aliases)+len(p.RedundantEnvironments) > 0 {
			return errors.New("version 1 cannot reconcile aliases")
		}
		return nil
	}
	if len(p.Aliases) != len(p.Repairs) || len(p.CleanupJobs) > 0 || len(p.RedundantEnvironments) > 16 {
		return errors.New("version 2 requires one alias selection per repair and cannot supersede cleanup")
	}
	if err := p.validateAliasRows(); err != nil {
		return err
	}
	return p.validateAliasEnvironments()
}

func (p Plan) validateAliasRows() error {
	seen := map[string]bool{}
	count := 0
	for _, a := range p.Aliases {
		if a.WorktreeID == "" || seen[a.WorktreeID] || a.CanonicalRowID == "" || len(a.AliasRowIDs) == 0 {
			return errors.New("incomplete or duplicated alias selection")
		}
		seen[a.WorktreeID] = true
		rows := map[string]bool{a.CanonicalRowID: true}
		for _, id := range a.AliasRowIDs {
			if id == "" || rows[id] {
				return errors.New("alias row IDs must be distinct")
			}
			rows[id] = true
			count++
		}
	}
	if count > 256 {
		return errors.New("alias selection exceeds 256 rows")
	}
	for _, r := range p.Repairs {
		if !seen[r.WorktreeID] || r.SourcePath != r.Path {
			return errors.New("version 2 cannot relocate or omit selected identities")
		}
	}
	return nil
}

func (p Plan) validateAliasEnvironments() error {
	envs := map[string]bool{}
	sessions := 0
	for _, e := range p.RedundantEnvironments {
		if e.EnvironmentID == "" || e.CanonicalEnvironmentID == "" || e.GroupID == "" || e.EnvironmentID == e.CanonicalEnvironmentID || envs[e.EnvironmentID] {
			return errors.New("invalid redundant environment selection")
		}
		envs[e.EnvironmentID] = true
		sessions += len(e.SessionIDs)
	}
	if sessions > 256 {
		return errors.New("alias selection exceeds 256 sessions")
	}
	return nil
}

const (
	aliasSessionTable     = "task_sessions"
	aliasStatusActive     = "active"
	aliasStatusFailed     = "failed"
	aliasStatusCancelled  = "cancelled"
	aliasEnvironmentTable = "task_environments"
	aliasSlotTable        = "task_environment_repos"
)

const (
	environmentIDColumn = "task_environment_id"
	sessionWaitingState = "WAITING_FOR_INPUT"
	snapshotTable       = "task_session_git_snapshots"
	cleanupJobTable     = "task_resource_cleanup_jobs"
)
