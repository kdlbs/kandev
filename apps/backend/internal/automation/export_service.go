package automation

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jmoiron/sqlx"
	"gopkg.in/yaml.v3"
)

// ErrExportWorkspaceLookupUnavailable is returned when AC-44 step 2's
// workspace-existence check has no lookup wired via
// SetExportWorkspaceLookup. Distinct from repoerrors.ErrWorkspaceNotFound so
// the HTTP layer's errors.Is classification (AC-44) reports this as a 500,
// never a 404 — an unwired lookup is a construction error, not a deployment
// mode.
var ErrExportWorkspaceLookupUnavailable = errors.New("automation: export workspace lookup is not available")

// ExportAutomationsDocument renders workspaceID's automations as the single
// YAML export document (the non-zip form of AC-1 through AC-49), applying
// AC-44's three-step evaluation order. The returned error classifies via
// errors.Is(err, repoerrors.ErrWorkspaceNotFound) for the HTTP layer's 404
// (AC-31, AC-35); any other non-nil error is a 500 (AC-45).
func (s *Service) ExportAutomationsDocument(ctx context.Context, workspaceID string) ([]byte, error) {
	entries, warnings, err := s.collectExportAutomations(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	automations := make([]exportAutomation, len(entries))
	for i, e := range entries {
		automations[i] = e.Automation
	}
	body, err := marshalExportDocument(newExportDocument(automations, warnings))
	if err != nil {
		return nil, fmt.Errorf("marshal export document: %w", err)
	}
	return body, nil
}

// ExportAutomationsZip renders workspaceID's automations as the zip export
// (AC-24 through AC-28), applying the same AC-44 evaluation order and error
// classification as ExportAutomationsDocument.
func (s *Service) ExportAutomationsZip(ctx context.Context, workspaceID string) ([]byte, error) {
	entries, warnings, err := s.collectExportAutomations(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	body, err := buildAutomationZip(entries, warnings)
	if err != nil {
		return nil, fmt.Errorf("build export zip: %w", err)
	}
	return body, nil
}

// collectExportAutomations implements AC-44's three-step evaluation
// (authorize, resolve workspace existence, export) and AC-29's
// single-transaction orchestration: every automation is read and assembled
// within one read transaction whose snapshot establishes at the first read
// (AC-13, AC-30). The result is sorted by automation name ascending,
// tiebroken by id ascending (AC-8), with warnings deduped, ordered, and
// rendered by buildWarningsList.
func (s *Service) collectExportAutomations(ctx context.Context, workspaceID string) ([]automationZipEntry, []string, error) {
	if err := s.authorizeWs(ctx, workspaceID); err != nil {
		return nil, nil, err
	}
	if s.exportWorkspaceLookup == nil {
		return nil, nil, ErrExportWorkspaceLookupUnavailable
	}
	if err := s.exportWorkspaceLookup.WorkspaceExists(ctx, workspaceID); err != nil {
		return nil, nil, err
	}

	tx, err := s.store.BeginReadTx(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin export read tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	automations, err := s.store.ListAutomationsForExportTx(ctx, tx, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("list automations for export: %w", err)
	}

	entries := make([]automationZipEntry, 0, len(automations))
	var warnings []exportWarning
	for _, a := range automations {
		exp, aWarnings, err := s.buildExportAutomation(ctx, tx, a)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, aWarnings...)
		entries = append(entries, automationZipEntry{ID: a.ID, Name: a.Name, Automation: exp})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].ID < entries[j].ID
	})

	return entries, buildWarningsList(warnings), nil
}

// buildExportAutomation assembles one automation's exportAutomation value:
// resolved descriptors (resolveDescriptors), its prompt node (AC-4 omits the
// key entirely for an empty prompt, before any fidelity rule applies), and
// each trigger's config node, with triggers ordered by type then id (AC-8).
func (s *Service) buildExportAutomation(ctx context.Context, tx *sqlx.Tx, a *Automation) (exportAutomation, []exportWarning, error) {
	resolved, warnings, err := s.resolveDescriptors(ctx, tx, a)
	if err != nil {
		return exportAutomation{}, nil, fmt.Errorf("resolve descriptors for automation %q: %w", a.ID, err)
	}

	var promptNode *yaml.Node
	if a.Prompt != "" {
		node, warning, err := buildPromptNode(a.Prompt)
		if err != nil {
			return exportAutomation{}, nil, fmt.Errorf("build prompt for automation %q: %w", a.ID, err)
		}
		promptNode = node
		if warning != "" {
			warnings = append(warnings, exportWarning{AutomationName: a.Name, AutomationID: a.ID, DedupKey: a.ID, Message: warning})
		}
	}

	triggers, triggerWarnings, err := s.buildExportTriggers(a)
	if err != nil {
		return exportAutomation{}, nil, err
	}
	warnings = append(warnings, triggerWarnings...)

	return exportAutomation{
		Name:              a.Name,
		Description:       a.Description,
		Enabled:           a.Enabled,
		MaxConcurrentRuns: a.MaxConcurrentRuns,
		ContinuationPolicy: func() ContinuationPolicy {
			if a.ContinuationPolicy == "" {
				return ContinuationPolicyNewTask
			}
			return a.ContinuationPolicy
		}(),
		TaskMode: func() TaskMode {
			if a.TaskMode == "" {
				return TaskModeAutomationRun
			}
			return a.TaskMode
		}(),
		RepositoryMode: func() RepositoryMode {
			if a.RepositoryMode == "" {
				if len(a.RepositoryIDs) > 0 {
					return RepositoryModeSelected
				}
				return RepositoryModeNone
			}
			return a.RepositoryMode
		}(),
		TaskTitleTemplate: a.TaskTitleTemplate,
		Prompt:            promptNode,
		AgentProfile:      resolved.AgentProfile,
		ExecutorProfile:   resolved.ExecutorProfile,
		Workflow:          resolved.Workflow,
		Repositories:      resolved.Repositories,
		Triggers:          triggers,
	}, warnings, nil
}

// buildExportTriggers orders a's triggers by type ascending, tiebroken by id
// ascending (AC-8), and builds each one's exported form, including its
// AC-39 config-validation warning when its stored config fails the
// UTF-8/JSON/object-shape checks.
func (s *Service) buildExportTriggers(a *Automation) ([]exportTrigger, []exportWarning, error) {
	sorted := append([]AutomationTrigger(nil), a.Triggers...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type < sorted[j].Type
		}
		return sorted[i].ID < sorted[j].ID
	})

	triggers := make([]exportTrigger, 0, len(sorted))
	var warnings []exportWarning
	for _, trig := range sorted {
		node, warning, err := buildTriggerConfigNode(trig.Config)
		if err != nil {
			return nil, nil, fmt.Errorf("build trigger config for trigger %q: %w", trig.ID, err)
		}
		if warning != "" {
			warnings = append(warnings, newTriggerConfigWarning(a, trig, warning))
		}
		triggers = append(triggers, exportTrigger{Type: string(trig.Type), Enabled: trig.Enabled, Config: node})
	}
	return triggers, warnings, nil
}
