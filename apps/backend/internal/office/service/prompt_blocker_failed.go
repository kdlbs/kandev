package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/office/wakeup"
)

// maxBlockerFailedDependents bounds the held-dependent list rendered into a
// task_blocker_failed prompt; the remainder is summarized as a count.
const maxBlockerFailedDependents = 20

// BlockerFailurePrompt is the task_blocker_failed wake context.
type BlockerFailurePrompt struct {
	Blocker      BlockerFailureTask
	Parent       BlockerFailureTask
	BlockerState string
	Held         []BlockerFailureTask
	HeldOmitted  int
}

// BlockerFailureTask is one task named by a task_blocker_failed prompt.
type BlockerFailureTask struct {
	ID         string
	Identifier string
	Title      string
}

func (t BlockerFailureTask) ref() string {
	id := t.Identifier
	if id == "" {
		id = t.ID
	}
	id = capRunes(sanitizePromptField(id), maxChildIdentifierRunes)
	if id == "" {
		id = "?"
	}
	title := capRunes(sanitizePromptField(t.Title), maxChildTitleRunes)
	if title == "" {
		return "[" + id + "]"
	}
	return "[" + id + "] " + title
}

func (si *SchedulerIntegration) enrichBlockerFailedContext(
	ctx context.Context, pc *PromptContext, payload, contextSnapshot string,
) {
	alerts := make(map[string]wakeup.BlockerFailureAlert)
	for _, raw := range []string{payload, contextSnapshot} {
		var p wakeup.BlockerFailurePayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		for occurrenceID, alert := range p.Alerts {
			alerts[occurrenceID] = alert
		}
		if len(p.Alerts) == 0 && p.BlockerTaskID != "" {
			key := "legacy:" + p.BlockerTaskID + ":" + p.BlockerState
			if _, exists := alerts[key]; !exists {
				alerts[key] = wakeup.BlockerFailureAlert{
					ParentTaskID: p.TaskID, BlockerTaskID: p.BlockerTaskID,
					BlockerState: p.BlockerState, HeldDependentIDs: p.HeldDependentIDs,
				}
			}
		}
	}
	keys := make([]string, 0, len(alerts))
	for key := range alerts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		alert := alerts[key]
		bf := BlockerFailurePrompt{
			Blocker:      si.blockerFailureTask(ctx, alert.BlockerTaskID),
			BlockerState: alert.BlockerState,
		}
		if alert.ParentTaskID != "" {
			bf.Parent = si.blockerFailureTask(ctx, alert.ParentTaskID)
		}
		for i, id := range alert.HeldDependentIDs {
			if i == maxBlockerFailedDependents {
				bf.HeldOmitted = len(alert.HeldDependentIDs) - i
				break
			}
			bf.Held = append(bf.Held, si.blockerFailureTask(ctx, id))
		}
		if pc.BlockerFailure == nil {
			pc.BlockerFailure = &bf
		} else {
			pc.AdditionalBlockerFailures = append(pc.AdditionalBlockerFailures, bf)
		}
	}
}

func (si *SchedulerIntegration) blockerFailureTask(ctx context.Context, id string) BlockerFailureTask {
	task := BlockerFailureTask{ID: id}
	if info, err := si.svc.repo.GetTaskBasicInfo(ctx, id); err == nil && info != nil {
		task.Identifier, task.Title = info.Identifier, info.Title
	}
	return task
}

// buildBlockerFailedPrompt tells the responsible agent which blocker failed
// and which dependents it now holds.
func buildBlockerFailedPrompt(pc *PromptContext) string {
	if pc.BlockerFailure == nil && len(pc.AdditionalBlockerFailures) == 0 {
		return "A blocker task failed or was cancelled, and its dependents remain held."
	}
	var b strings.Builder
	alerts := make([]BlockerFailurePrompt, 0, 1+len(pc.AdditionalBlockerFailures))
	if pc.BlockerFailure != nil {
		alerts = append(alerts, *pc.BlockerFailure)
	}
	alerts = append(alerts, pc.AdditionalBlockerFailures...)
	for i, bf := range alerts {
		if i > 0 {
			b.WriteString("\nAdditional blocker failure:\n")
		}
		state := strings.ToLower(bf.BlockerState)
		if state == "" {
			state = "failed"
		}
		if bf.Parent.ID != "" {
			fmt.Fprintf(&b, "Coordinator task %s.\n", bf.Parent.ref())
		}
		fmt.Fprintf(&b, "Blocker task %s is %s.\n", bf.Blocker.ref(), state)
		b.WriteString("These dependent tasks remain held behind it:\n")
		for _, task := range bf.Held {
			fmt.Fprintf(&b, "- %s\n", task.ref())
		}
		if bf.HeldOmitted > 0 {
			fmt.Fprintf(&b, "- (+%d more)\n", bf.HeldOmitted)
		}
		b.WriteString("They stay held until the blocker is retried to completion, replaced, " +
			"or its dependency edge is removed. Decide how to repair the chain.")
		if i < len(alerts)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
