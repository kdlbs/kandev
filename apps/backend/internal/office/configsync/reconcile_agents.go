package configsync

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

const kindAgent = "agent"

// buildFetchedAgents parses every agents/*.yml file, resolves
// AC-OFFICE-CONFIG-SYNC-003.2 filename-stem warnings and
// AC-OFFICE-CONFIG-SYNC-003.3 key collisions, and returns the
// collision-resolved fetched set keyed by declared name, every reports_to
// name declared (for the second pass), any warnings, and the path of every
// file that failed to parse — AC-OFFICE-CONFIG-SYNC-003.12's deletion-sweep
// exemption input, resolved by the caller against the manifest.
func buildFetchedAgents(files []fetchedFile) (
	fetched []fetchedEntity[sqlite.AgentInstanceConfigFields], reportsTo map[string]string,
	warnings, unparsed []string,
) {
	type parsed struct {
		agent *parsedAgent
		path  string
	}
	var ok []parsed
	for _, f := range files {
		pa, err := parseAgentFile(f.path, f.content)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("agent file %q: %v; skipping", f.path, err))
			unparsed = append(unparsed, f.path)
			continue
		}
		if w := stemMismatchWarning(kindAgent, f.path, pa.declaredName); w != "" {
			warnings = append(warnings, w)
		}
		ok = append(ok, parsed{agent: pa, path: f.path})
	}

	keyed := make([]keyedPath, len(ok))
	byPath := make(map[string]*parsedAgent, len(ok))
	for i, p := range ok {
		keyed[i] = keyedPath{Key: p.agent.declaredName, Path: p.path}
		byPath[p.path] = p.agent
	}
	winners, collisionWarnings := resolveKeyCollisions(kindAgent, keyed)
	warnings = append(warnings, collisionWarnings...)

	reportsTo = map[string]string{}
	for key, winnerPath := range winners {
		pa := byPath[winnerPath]
		fetched = append(fetched, fetchedEntity[sqlite.AgentInstanceConfigFields]{
			Key:        key,
			SourcePath: winnerPath,
			Projection: sqlite.AgentInstanceConfigFields{
				Role:                  pa.role,
				Icon:                  pa.icon,
				BudgetMonthlyCents:    pa.budgetMonthlyCents,
				MaxConcurrentSessions: pa.maxConcurrentSessions,
				DesiredSkills:         normalizeJSONArrayForComparison(pa.desiredSkills),
				ExecutorPreference:    pa.executorPreference,
			},
		})
		if pa.reportsTo != "" {
			reportsTo[key] = pa.reportsTo
		}
	}
	return fetched, reportsTo, warnings, unparsed
}

// normalizeJSONArrayForComparison mirrors the sqlite package's own
// normalizeAgentJSONArray (empty string -> "[]") so a fetched projection
// compares equal to what a prior sync run actually persisted, instead of
// diffing forever against the writer's own normalization
// (AC-OFFICE-CONFIG-SYNC-003.11's idempotency requirement).
func normalizeJSONArrayForComparison(s string) string {
	if strings.TrimSpace(s) == "" {
		return "[]"
	}
	return s
}

// agentOps adapts the office repository's agent CRUD to the generic
// six-case apply engine (AC-OFFICE-CONFIG-SYNC-003.5c: agent's owned writer
// is UpdateAgentInstanceConfigFields).
//
// defaultAgentID is looked up once here, before any of this apply pass's
// per-entity transactions open — CreateAgentInstanceTx's own doc comment
// warns that its FK-inheritance lookup cannot safely repeat inside a
// transaction because it would not see the transaction's own uncommitted
// writes, and the same plain read deadlocks against a single-connection
// SQLite handle if it runs while a write transaction already holds that
// connection (as it would if this were called from inside the create
// closure instead).
func agentOps(ctx context.Context, repo *sqlite.Repository, workspaceID string) entityOps[sqlite.AgentInstanceConfigFields] {
	defaultAgentID := repo.DefaultAgentID(ctx, workspaceID)
	return entityOps[sqlite.AgentInstanceConfigFields]{
		kind: kindAgent,
		list: func(ctx context.Context, workspaceID string) ([]entityRow[sqlite.AgentInstanceConfigFields], error) {
			agents, err := repo.ListAgentInstances(ctx, workspaceID)
			if err != nil {
				return nil, err
			}
			rows := make([]entityRow[sqlite.AgentInstanceConfigFields], len(agents))
			for i, a := range agents {
				rows[i] = entityRow[sqlite.AgentInstanceConfigFields]{
					ID:  a.ID,
					Key: a.Name,
					Projection: sqlite.AgentInstanceConfigFields{
						Role:                  string(a.Role),
						Icon:                  a.Icon,
						BudgetMonthlyCents:    a.BudgetMonthlyCents,
						MaxConcurrentSessions: a.MaxConcurrentSessions,
						DesiredSkills:         normalizeJSONArrayForComparison(a.DesiredSkills),
						ExecutorPreference:    a.ExecutorPreference,
					},
				}
			}
			return rows, nil
		},
		create: func(
			ctx context.Context, tx *sqlx.Tx, workspaceID, key, _ string, proj sqlite.AgentInstanceConfigFields,
		) (string, error) {
			agent := &models.AgentInstance{
				ID:                    uuid.New().String(),
				WorkspaceID:           workspaceID,
				Name:                  key,
				AgentID:               defaultAgentID,
				Role:                  settingsmodels.AgentRole(proj.Role),
				Icon:                  proj.Icon,
				BudgetMonthlyCents:    proj.BudgetMonthlyCents,
				MaxConcurrentSessions: proj.MaxConcurrentSessions,
				DesiredSkills:         proj.DesiredSkills,
				ExecutorPreference:    proj.ExecutorPreference,
			}
			if err := repo.CreateAgentInstanceTx(ctx, tx, agent); err != nil {
				return "", err
			}
			return agent.ID, nil
		},
		update: func(ctx context.Context, tx *sqlx.Tx, id string, proj sqlite.AgentInstanceConfigFields) error {
			return repo.UpdateAgentInstanceConfigFieldsTx(ctx, tx, id, proj)
		},
		del: func(ctx context.Context, tx *sqlx.Tx, id string) error {
			return repo.DeleteAgentInstanceTx(ctx, tx, id)
		},
	}
}

// maxReportsToHops bounds the cycle-detection walk in
// resolveAgentReportsTo — a defensive cap, not an expected depth.
const maxReportsToHops = 50

// resolveAgentReportsTo runs config sync's second pass
// (AC-OFFICE-CONFIG-SYNC-003.9a/.9b/.9c, .10/.10b): reports_to is owned by
// sync like any other field on an applied agent (AC-OFFICE-CONFIG-SYNC-003.5d),
// so every agent this run created or confirmed existing (idsByKey) gets its
// field resolved and written, not only the ones with a currently non-empty
// declaration — an agent whose file dropped the field, or replaced it with a
// self-reference or a cycle, must have a stale DB value cleared, not left in
// place. A name outside idsByKey (an agent not managed by this run's fetched
// set), a self-reference, or a cycle resolves to empty with a warning instead
// of a target ID — allowExternalManagers is always false. unappliedKeys is
// the union of buildKindsFetch's agent deletion-sweep exemption set
// (kindExemptions: manifest entity keys whose file was unreadable or failed
// to parse this run) and this pass's own decisionForeign losers (fetched
// but left untouched because an unmanaged entity already held the name),
// reused here to distinguish "fetched but not applied" from a target that
// names no agent at all (AC-OFFICE-CONFIG-SYNC-003.10b).
func resolveAgentReportsTo(
	ctx context.Context, repo *sqlite.Repository, reportsTo map[string]string,
	idsByKey map[string]string, unappliedKeys map[string]bool,
) ([]string, error) {
	keys := make([]string, 0, len(idsByKey))
	for key := range idsByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var warnings []string
	for _, key := range keys {
		id := idsByKey[key]
		targetID, warning := resolveOneAgentReportsTo(key, reportsTo, idsByKey, unappliedKeys)
		if warning != "" {
			warnings = append(warnings, warning)
		}
		if err := repo.UpdateAgentReportsTo(ctx, id, targetID); err != nil {
			return warnings, fmt.Errorf("agent %q: set reports_to: %w", key, err)
		}
	}
	return warnings, nil
}

// resolveOneAgentReportsTo resolves one agent's reports_to declaration
// against this run's fetched set, returning the target entity ID to write
// (empty when the field must be cleared) and, when clearing is not simply
// "no declaration", the warning explaining why. AC-OFFICE-CONFIG-SYNC-003.10b
// requires a target that was fetched but not applied this run (its file was
// unreadable or failed to parse, or it lost a same-name collision to an
// unmanaged entity) to be warned distinctly from a target that names no
// agent anywhere; unappliedKeys carries that signal.
func resolveOneAgentReportsTo(key string, reportsTo, idsByKey map[string]string, unappliedKeys map[string]bool) (targetID, warning string) {
	target, declared := reportsTo[key]
	switch {
	case !declared:
		return "", ""
	case target == key:
		return "", fmt.Sprintf("agent %q: reports_to is a self-reference; leaving unset", key)
	case detectReportsToCycle(key, target, reportsTo):
		return "", fmt.Sprintf("agent %q: reports_to %q would create a cycle; leaving unset", key, target)
	}
	if targetID, ok := idsByKey[target]; ok {
		return targetID, ""
	}
	if unappliedKeys[target] {
		return "", fmt.Sprintf(
			"agent %q: reports_to %q was fetched but not applied this run (its file was unreadable, failed to parse, "+
				"or it lost a name collision to an unmanaged agent); leaving unset",
			key, target)
	}
	return "", fmt.Sprintf("agent %q: reports_to %q is not managed by this sync; leaving unset", key, target)
}

// detectReportsToCycle walks the reports_to chain starting at target,
// bounded by maxReportsToHops, and reports whether it loops back to key. A
// revisit of any node other than key means the chain has wandered into a
// cycle that does not involve key (someone else's problem, resolved when
// their own reports_to is checked) and is not a cycle for key, so only
// cur == key ends the walk positively; hitting the hop cap without
// returning to key is not a cycle either, mirroring office/config's
// reportsToCycleExists (import.go), which checks node == target the same
// way.
func detectReportsToCycle(key, target string, reportsTo map[string]string) bool {
	visited := map[string]bool{}
	cur := target
	for range maxReportsToHops {
		if cur == key {
			return true
		}
		if visited[cur] {
			return false
		}
		visited[cur] = true
		next, ok := reportsTo[cur]
		if !ok {
			return false
		}
		cur = next
	}
	return false
}
