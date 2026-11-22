package workflowsync

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

// PromptReferenceChecker reports, per text, the @name references that do not
// resolve to a saved prompt. Satisfied by the prompt service.
type PromptReferenceChecker interface {
	UnresolvedPromptReferences(ctx context.Context, texts []string) ([][]string, error)
}

// SetPromptReferenceChecker enables unresolved-reference warnings. A nil
// checker disables them.
func (s *Service) SetPromptReferenceChecker(checker PromptReferenceChecker) {
	s.promptReferences = checker
}

// referenceSite is one prompt text in a parsed definition. An empty step
// marks the workflow-level prompt.
type referenceSite struct {
	path, workflow, step string
}

type missingReference struct {
	site           referenceSite
	name           string
	workflowPrompt bool
	steps          []string
}

// unresolvedReferenceWarnings reports one warning per (file, workflow,
// missing name) across the workflow prompt and step prompts of every parsed
// definition. Files that failed to parse are not checked. A checker failure
// becomes a single warning and never fails the sync.
func (s *Service) unresolvedReferenceWarnings(ctx context.Context, files []workflowservice.SyncFileExport) []string {
	if s.promptReferences == nil {
		return nil
	}
	sites, texts := collectReferenceSites(files)
	if len(texts) == 0 {
		return nil
	}
	missing, err := s.promptReferences.UnresolvedPromptReferences(ctx, texts)
	if err != nil {
		s.logger.Warn("failed to check workflow prompt references", zap.Error(err))
		return []string{"workflow sync could not check saved-prompt references; sync again to retry the check"}
	}
	return formatMissingReferences(groupMissingReferences(sites, missing))
}

func collectReferenceSites(files []workflowservice.SyncFileExport) ([]referenceSite, []string) {
	var sites []referenceSite
	var texts []string
	for _, file := range files {
		if file.Export == nil {
			continue
		}
		for _, wf := range file.Export.Workflows {
			if strings.Contains(wf.Prompt, "@") {
				sites = append(sites, referenceSite{path: file.Path, workflow: wf.Name})
				texts = append(texts, wf.Prompt)
			}
			for _, step := range stepsByPosition(wf.Steps) {
				if strings.Contains(step.Prompt, "@") {
					sites = append(sites, referenceSite{path: file.Path, workflow: wf.Name, step: step.Name})
					texts = append(texts, step.Prompt)
				}
			}
		}
	}
	return sites, texts
}

func stepsByPosition(steps []workflowmodels.StepPortable) []workflowmodels.StepPortable {
	sorted := append([]workflowmodels.StepPortable(nil), steps...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })
	return sorted
}

func groupMissingReferences(sites []referenceSite, missing [][]string) []*missingReference {
	var ordered []*missingReference
	index := map[string]*missingReference{}
	for i, names := range missing {
		if i >= len(sites) {
			break
		}
		site := sites[i]
		for _, name := range names {
			key := site.path + "\x00" + site.workflow + "\x00" + name
			group := index[key]
			if group == nil {
				group = &missingReference{site: referenceSite{path: site.path, workflow: site.workflow}, name: name}
				index[key] = group
				ordered = append(ordered, group)
			}
			if site.step == "" {
				group.workflowPrompt = true
			} else {
				group.steps = append(group.steps, site.step)
			}
		}
	}
	return ordered
}

func formatMissingReferences(groups []*missingReference) []string {
	warnings := make([]string, 0, len(groups))
	for _, group := range groups {
		var where []string
		if group.workflowPrompt {
			where = append(where, "its workflow prompt")
		}
		if len(group.steps) > 0 {
			quoted := make([]string, len(group.steps))
			for i, step := range group.steps {
				quoted[i] = fmt.Sprintf("%q", step)
			}
			where = append(where, "step(s) "+strings.Join(quoted, ", "))
		}
		warnings = append(warnings, fmt.Sprintf("%s: workflow %q references @%s in %s, which is not a saved prompt",
			group.site.path, group.site.workflow, group.name, strings.Join(where, " and ")))
	}
	return warnings
}
