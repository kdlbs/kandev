package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	turnTitleRenameTitle        = "Renamed turn continuity"
	turnTitleRenameFile         = "turn-title-rename.txt"
	turnTitleRenameContent      = "renamed turn edit\n"
	turnTitleRenameSwitchName   = "kandev-title-rename-switch"
	turnTitleRenameOnlyMarker   = "turn-title-rename-only complete"
	turnTitleRenameEditMarker   = "turn-title-rename-edit complete"
	turnTitleRenameSwitchMarker = "turn-title-rename-switch complete"
)

type turnTitleRenameMode int

const (
	turnTitleRenameOnly turnTitleRenameMode = iota
	turnTitleRenameWithEdit
	turnTitleRenameThenSwitch
)

// turnTitleRenameAgent is the slice of the emitter the scenario needs. Tests
// substitute a caller that records the real MCP invocation order.
type turnTitleRenameAgent interface {
	text(string)
	callMCPToolCtx(ctx context.Context, serverName, toolName string, args map[string]any) (string, error)
}

func scenarioTurnTitleRenameOnly(e *emitter) {
	runTurnTitleRenameScenario(e, turnTitleRenameOnly)
}

func scenarioTurnTitleRenameEdit(e *emitter) {
	runTurnTitleRenameScenario(e, turnTitleRenameWithEdit)
}

func scenarioTurnTitleRenameSwitch(e *emitter) {
	runTurnTitleRenameScenario(e, turnTitleRenameThenSwitch)
}

// runTurnTitleRenameScenario resolves the title through the real MCP tool,
// verifies the accepted generated-branch rename, and only then performs the
// scenario's optional filesystem or branch mutation.
func runTurnTitleRenameScenario(agent turnTitleRenameAgent, mode turnTitleRenameMode) {
	wd, err := os.Getwd()
	if err != nil {
		agent.text("turn-title-rename: working directory unavailable")
		return
	}
	result, err := agent.callMCPToolCtx(context.Background(), "kandev", "set_task_title_kandev",
		map[string]any{"title": turnTitleRenameTitle})
	if err != nil {
		agent.text("turn-title-rename: title tool failed: " + err.Error())
		return
	}
	if _, err := verifyTurnTitleRenameResult(wd, result); err != nil {
		agent.text("turn-title-rename: " + err.Error())
		return
	}
	switch mode {
	case turnTitleRenameWithEdit:
		if err := writeGitFile(turnTitleRenameFile, turnTitleRenameContent); err != nil {
			agent.text("turn-title-rename: write failed: " + err.Error())
			return
		}
	case turnTitleRenameThenSwitch:
		if err := switchAfterTurnTitleRename(wd); err != nil {
			agent.text("turn-title-rename: branch switch failed: " + err.Error())
			return
		}
	}
	agent.text(turnTitleRenameMarker(mode))
}

func turnTitleRenameMarker(mode turnTitleRenameMode) string {
	switch mode {
	case turnTitleRenameWithEdit:
		return turnTitleRenameEditMarker
	case turnTitleRenameThenSwitch:
		return turnTitleRenameSwitchMarker
	default:
		return turnTitleRenameOnlyMarker
	}
}

type turnTitleRenameResult struct {
	Accepted     bool `json:"accepted"`
	BranchRename struct {
		Status  string `json:"status"`
		Renamed []struct {
			RepositoryID string `json:"repository_id"`
			To           string `json:"to"`
		} `json:"renamed"`
		Preserved []struct {
			Reason string `json:"reason"`
		} `json:"preserved"`
		Failed []struct {
			RepositoryID string `json:"repository_id"`
			Message      string `json:"message"`
		} `json:"failed"`
	} `json:"branch_rename"`
}

func verifyTurnTitleRenameResult(wd, raw string) (string, error) {
	payload := turnTitleRenameResult{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", fmt.Errorf("title result is malformed: %w", err)
	}
	if !payload.Accepted {
		return "", errors.New("title was not accepted")
	}
	if payload.BranchRename.Status != "renamed" || len(payload.BranchRename.Renamed) == 0 {
		detail := "no renamed repository"
		if len(payload.BranchRename.Failed) > 0 {
			detail = payload.BranchRename.Failed[0].Message
		}
		return "", fmt.Errorf(
			"generated branch rename was not verified: %s (renamed=%d failed=%d preserved=%d detail=%s)",
			payload.BranchRename.Status, len(payload.BranchRename.Renamed),
			len(payload.BranchRename.Failed), len(payload.BranchRename.Preserved), detail)
	}
	renamedTo := payload.BranchRename.Renamed[0].To
	branchOut, err := runGitOutput(wd, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	if current := strings.TrimSpace(string(branchOut)); current != renamedTo {
		return "", fmt.Errorf("current branch %q does not match the verified rename %q", current, renamedTo)
	}
	return renamedTo, nil
}

func switchAfterTurnTitleRename(wd string) error {
	headBefore, err := runGitOutput(wd, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	git := makeGitRunner(wd)
	if err := git("checkout", "-q", "-b", turnTitleRenameSwitchName); err != nil {
		return err
	}
	headAfter, err := runGitOutput(wd, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(headAfter)) != strings.TrimSpace(string(headBefore)) {
		return errors.New("branch switch moved HEAD")
	}
	branchOut, err := runGitOutput(wd, "branch", "--show-current")
	if err != nil {
		return err
	}
	if current := strings.TrimSpace(string(branchOut)); current != turnTitleRenameSwitchName {
		return fmt.Errorf("branch switch landed on %q", current)
	}
	return nil
}
