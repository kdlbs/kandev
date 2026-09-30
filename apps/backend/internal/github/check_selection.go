package github

import (
	"fmt"
	"strings"
)

const githubActionsAppID int64 = 15368

type selectedPRChecks struct {
	Checks             []CheckRun
	ActiveWorkflowRuns bool
}

type workflowSuiteSelection struct {
	selectedBySuite    map[int64][]WorkflowRun
	supersededSuites   map[int64]struct{}
	runsBySuite        map[int64][]WorkflowRun
	hasActiveWorkflows bool
}

func selectCurrentPRChecks(pr *PR, checks []CheckRun, runs []WorkflowRun) selectedPRChecks {
	workflowSelection := selectWorkflowSuites(pr, runs)
	checksForReduction := selectChecksFromWorkflowSuites(checks, workflowSelection)
	return selectedPRChecks{
		Checks:             reduceSelectedChecks(checksForReduction),
		ActiveWorkflowRuns: workflowSelection.hasActiveWorkflows,
	}
}

func selectWorkflowSuites(pr *PR, runs []WorkflowRun) workflowSuiteSelection {
	selection := workflowSuiteSelection{
		selectedBySuite:  make(map[int64][]WorkflowRun),
		supersededSuites: make(map[int64]struct{}),
		runsBySuite:      workflowRunsBySuiteForPRHead(pr, runs),
	}
	matchedByGroup := matchedWorkflowRunsByGroup(pr, runs)
	for _, groupedRuns := range matchedByGroup {
		selection.addMatchedRunGroup(groupedRuns)
	}
	for suiteID, selected := range selection.selectedBySuite {
		selection.runsBySuite[suiteID] = append(selection.runsBySuite[suiteID], selected...)
	}
	return selection
}

func matchedWorkflowRunsByGroup(pr *PR, runs []WorkflowRun) map[string][]WorkflowRun {
	matchedByGroup := make(map[string][]WorkflowRun)
	for _, run := range runs {
		if !workflowRunMatchesForCheckSelection(run, pr) {
			continue
		}
		key := workflowRunGroupKey(run)
		matchedByGroup[key] = append(matchedByGroup[key], run)
	}
	return matchedByGroup
}

func workflowRunsBySuiteForPRHead(pr *PR, runs []WorkflowRun) map[int64][]WorkflowRun {
	runsBySuite := make(map[int64][]WorkflowRun)
	for _, run := range runs {
		if pr == nil || run.CheckSuiteID == 0 || run.HeadSHA != pr.HeadSHA {
			continue
		}
		runsBySuite[run.CheckSuiteID] = append(runsBySuite[run.CheckSuiteID], run)
	}
	return runsBySuite
}

func (selection *workflowSuiteSelection) addMatchedRunGroup(group []WorkflowRun) {
	current := newestWorkflowRun(group)
	if workflowRunIsActive(current) {
		selection.hasActiveWorkflows = true
	}
	if current.CheckSuiteID == 0 {
		return
	}
	selection.selectedBySuite[current.CheckSuiteID] = append(selection.selectedBySuite[current.CheckSuiteID], current)
	for _, earlier := range group {
		if earlier.ID == current.ID || earlier.CheckSuiteID == 0 || earlier.CheckSuiteID == current.CheckSuiteID {
			continue
		}
		selection.supersededSuites[earlier.CheckSuiteID] = struct{}{}
	}
}

func newestWorkflowRun(runs []WorkflowRun) WorkflowRun {
	current := runs[0]
	for _, candidate := range runs[1:] {
		if workflowRunNewer(candidate, current) {
			current = candidate
		}
	}
	return current
}

func selectChecksFromWorkflowSuites(checks []CheckRun, selection workflowSuiteSelection) []CheckRun {
	checksForReduction := make([]CheckRun, 0, len(checks))
	for _, check := range checks {
		if isSupersededWorkflowCheck(check, selection) {
			continue
		}
		check = attachUniqueWorkflowRun(check, selection.runsBySuite)
		checksForReduction = append(checksForReduction, check)
	}
	return checksForReduction
}

func isSupersededWorkflowCheck(check CheckRun, selection workflowSuiteSelection) bool {
	if check.CheckSuiteID == 0 {
		return false
	}
	if _, superseded := selection.supersededSuites[check.CheckSuiteID]; !superseded {
		return false
	}
	_, stillCurrent := selection.selectedBySuite[check.CheckSuiteID]
	return !stillCurrent
}

func attachUniqueWorkflowRun(check CheckRun, runsBySuite map[int64][]WorkflowRun) CheckRun {
	if check.CheckSuiteID == 0 {
		return check
	}
	if run, unique := uniqueWorkflowRunForSuite(runsBySuite[check.CheckSuiteID]); unique {
		return attachWorkflowRunIdentity(check, run)
	}
	return check
}

func uniqueWorkflowRunForSuite(runs []WorkflowRun) (WorkflowRun, bool) {
	if len(runs) == 0 {
		return WorkflowRun{}, false
	}
	byRunID := make(map[int64]WorkflowRun)
	for _, run := range runs {
		if existing, ok := byRunID[run.ID]; ok {
			if workflowRunNewer(run, existing) {
				byRunID[run.ID] = run
			}
			continue
		}
		byRunID[run.ID] = run
	}
	if len(byRunID) != 1 {
		return WorkflowRun{}, false
	}
	for _, run := range byRunID {
		return run, true
	}
	return WorkflowRun{}, false
}

func workflowRunMatchesForCheckSelection(run WorkflowRun, pr *PR) bool {
	if workflowRunMatchesPR(run, pr) {
		return true
	}
	if pr == nil || len(run.PullRequests) > 0 || !strings.EqualFold(run.Event, "pull_request_target") {
		return false
	}
	return run.HeadSHA != "" && run.HeadSHA == pr.HeadSHA &&
		run.HeadBranch != "" && run.HeadBranch == pr.HeadBranch &&
		workflowRepositoryIdentityMatchesPR(workflowRepositoryIdentityFromRun(run), pr)
}

func workflowRunIsActive(run WorkflowRun) bool {
	switch strings.ToLower(strings.TrimSpace(run.Status)) {
	case "queued", "in_progress", checkStatusPending, "requested", "waiting":
		return true
	default:
		return false
	}
}

func attachWorkflowRunIdentity(check CheckRun, run WorkflowRun) CheckRun {
	check.WorkflowID = run.WorkflowID
	check.WorkflowName = run.Name
	check.WorkflowRunID = run.ID
	check.WorkflowEvent = run.Event
	check.HeadRepoID = run.HeadRepoID
	check.HeadRepoOwner = run.HeadRepoOwner
	check.HeadRepoName = run.HeadRepoName
	check.HeadBranch = run.HeadBranch
	return check
}

func reduceSelectedChecks(checks []CheckRun) []CheckRun {
	selected := make([]CheckRun, 0, len(checks))
	byKey := make(map[string]int, len(checks))
	for _, check := range checks {
		if check.Source == checkSourceStatusContext {
			key := "status:" + checkMergeKey(check)
			if index, ok := byKey[key]; ok {
				if isNewerCheck(check, selected[index]) {
					selected[index] = check
				}
				continue
			}
			byKey[key] = len(selected)
			selected = append(selected, check)
			continue
		}

		key, deduplicate := selectedCheckKey(check)
		if !deduplicate {
			selected = append(selected, check)
			continue
		}
		if index, ok := byKey[key]; ok {
			if selectedCheckIsNewer(check, selected[index]) {
				selected[index] = check
			}
			continue
		}
		byKey[key] = len(selected)
		selected = append(selected, check)
	}
	return selected
}

func selectedCheckKey(check CheckRun) (string, bool) {
	name := checkMergeKey(check)
	if check.WorkflowRunID != 0 {
		repository := fmt.Sprintf("id:%d", check.HeadRepoID)
		if check.HeadRepoID == 0 {
			repository = strings.ToLower(check.HeadRepoOwner) + "/" + strings.ToLower(check.HeadRepoName)
		}
		return fmt.Sprintf("workflow:%d|run:%d|event:%s|repository:%s|branch:%s|app:%s|%s",
			check.WorkflowID, check.WorkflowRunID, strings.ToLower(check.WorkflowEvent), repository,
			check.HeadBranch, selectedCheckAppKey(check), name), true
	}
	if isGitHubActionsCheck(check) {
		if check.CheckSuiteID == 0 {
			return "", false
		}
		return fmt.Sprintf("actions-suite:%d|app:%s|%s", check.CheckSuiteID, selectedCheckAppKey(check), name), true
	}
	if check.AppID != 0 {
		return fmt.Sprintf("app:%d|%s", check.AppID, name), true
	}
	if slug := strings.ToLower(strings.TrimSpace(check.AppSlug)); slug != "" {
		return "app:" + slug + "|" + name, true
	}
	if check.CheckSuiteID != 0 {
		return fmt.Sprintf("suite:%d|%s", check.CheckSuiteID, name), true
	}
	if check.ID == 0 {
		return "legacy:" + name, true
	}
	return "", false
}

func isGitHubActionsCheck(check CheckRun) bool {
	return check.AppID == githubActionsAppID || strings.EqualFold(strings.TrimSpace(check.AppSlug), "github-actions")
}

func selectedCheckIsNewer(candidate, current CheckRun) bool {
	if candidate.ID != 0 && current.ID != 0 {
		return candidate.ID > current.ID
	}
	return isNewerCheck(candidate, current)
}

func selectedCheckAppKey(check CheckRun) string {
	if check.AppID != 0 {
		return fmt.Sprintf("id:%d", check.AppID)
	}
	return strings.ToLower(strings.TrimSpace(check.AppSlug))
}
