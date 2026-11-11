package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// maxBlockerFailedDependents bounds the held-dependent list rendered into a
// task_blocker_failed prompt; the remainder is summarized as a count.
const maxBlockerFailedDependents = 20

// BlockerFailurePrompt is the task_blocker_failed wake context.
type BlockerFailurePrompt struct {
	Blocker      BlockerFailureTask
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
	if t.Title == "" {
		return "[" + id + "]"
	}
	return "[" + id + "] " + t.Title
}

func (si *SchedulerIntegration) enrichBlockerFailedContext(
	ctx context.Context, pc *PromptContext, payload string,
) {
	var p struct {
		BlockerTaskID string   `json:"blocker_task_id"`
		BlockerState  string   `json:"blocker_state"`
		Held          []string `json:"held_dependent_ids"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil || p.BlockerTaskID == "" {
		return
	}
	bf := &BlockerFailurePrompt{Blocker: si.blockerFailureTask(ctx, p.BlockerTaskID), BlockerState: p.BlockerState}
	for i, id := range p.Held {
		if i == maxBlockerFailedDependents {
			bf.HeldOmitted = len(p.Held) - i
			break
		}
		bf.Held = append(bf.Held, si.blockerFailureTask(ctx, id))
	}
	pc.BlockerFailure = bf
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
	bf := pc.BlockerFailure
	if bf == nil {
		return "A blocker task failed or was cancelled, and its dependents remain held."
	}
	state := strings.ToLower(bf.BlockerState)
	if state == "" {
		state = "failed"
	}
	var b strings.Builder
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
	return b.String()
}
