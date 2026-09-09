package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/waveidentity"
)

// TestListStuckParents_WaveMatchBlocksAcrossNonMemberChildEdit_UnblockedOnWaveMemberChange
// is Task 06's core wave-comparison regression test
// (AC-OFFICE-WAKE-WAVE-IDENTITY-003): a delivered wave keeps blocking across
// a child-set change that doesn't touch wave membership (an automation-origin
// child completing), and only a real wave-member change (a genuine,
// non-automation, non-archived, non-ephemeral child) makes the parent a
// candidate again.
func TestListStuckParents_WaveMatchBlocksAcrossNonMemberChildEdit_UnblockedOnWaveMemberChange(t *testing.T) {
	for _, status := range []string{"finished", "failed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			repo := newSearchTestRepo(t)
			ctx := context.Background()

			const parentID = "parent-1"
			insertTask(t, repo, ctx, parentID, "ws-1", "Parent", "", "")
			if _, err := repo.ExecRaw(ctx,
				`UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, parentID,
			); err != nil {
				t.Fatalf("mark parent as Office task: %v", err)
			}
			child0 := parentID + "-child-0"
			insertTask(t, repo, ctx, child0, "ws-1", "Child", "", "")
			if _, err := repo.ExecRaw(ctx,
				`UPDATE tasks SET parent_id = ?, state = 'COMPLETED' WHERE id = ?`, parentID, child0,
			); err != nil {
				t.Fatalf("set child0 state: %v", err)
			}
			seedWakeAgentProfile(t, repo, ctx, parentID+"-agent", "idle")
			seedRunner(t, repo, ctx, parentID)

			waveString := waveidentity.WaveString(parentID, []string{child0})
			waveKey := waveidentity.WaveKey(parentID, []string{child0})
			seedWakeRunWithWave(t, repo, ctx, "run-1", parentID, "task_children_completed",
				status, "datetime('now', '+1 second')", waveKey, waveString)

			before, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
			if err != nil {
				t.Fatalf("ListStuckParents (delivered, unchanged wave): %v", err)
			}
			if len(before) != 0 {
				t.Fatalf("status %q: delivered unchanged wave must not be a candidate: %#v", status, before)
			}

			// A non-wave-member child completes: the child set changes, but
			// wave membership does not.
			automationChild := parentID + "-child-automation"
			insertTask(t, repo, ctx, automationChild, "ws-1", "Child", "", "")
			if _, err := repo.ExecRaw(ctx,
				`UPDATE tasks SET parent_id = ?, state = 'COMPLETED', origin = 'automation_run' WHERE id = ?`,
				parentID, automationChild,
			); err != nil {
				t.Fatalf("set automation child state: %v", err)
			}

			afterNonMemberEdit, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
			if err != nil {
				t.Fatalf("ListStuckParents (after non-member child edit): %v", err)
			}
			if len(afterNonMemberEdit) != 0 {
				t.Fatalf("status %q: a non-wave-member child edit must not unblock a matched wave: %#v",
					status, afterNonMemberEdit)
			}

			// A real wave member joins: wave membership itself changes.
			child1 := parentID + "-child-1"
			insertTask(t, repo, ctx, child1, "ws-1", "Child", "", "")
			if _, err := repo.ExecRaw(ctx,
				`UPDATE tasks SET parent_id = ?, state = 'COMPLETED' WHERE id = ?`, parentID, child1,
			); err != nil {
				t.Fatalf("set child1 state: %v", err)
			}

			afterWaveChange, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
			if err != nil {
				t.Fatalf("ListStuckParents (after wave-member change): %v", err)
			}
			if len(afterWaveChange) != 1 || afterWaveChange[0].ParentTaskID != parentID {
				t.Fatalf("status %q: a real wave-member change must unblock the parent: %#v",
					status, afterWaveChange)
			}
		})
	}
}

// TestListStuckParents_ExcludesParentWithOnlyNonWaveMemberChildren is
// AC-OFFICE-WAKE-WAVE-IDENTITY-003.9's regression test: a parent whose
// only children are archived, ephemeral, or automation-origin has no
// possible wave and must never be listed — the wave-member EXISTS gate is
// additive to (never a replacement for) the existing archived-only guard.
func TestListStuckParents_ExcludesParentWithOnlyNonWaveMemberChildren(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	// Ephemeral-only: one non-archived, non-automation child, but ephemeral.
	insertTask(t, repo, ctx, "parent-ephemeral-only", "ws-1", "Parent", "", "")
	insertTask(t, repo, ctx, "parent-ephemeral-only-child-0", "ws-1", "Child", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET parent_id = 'parent-ephemeral-only', state = 'COMPLETED', is_ephemeral = 1
		WHERE id = 'parent-ephemeral-only-child-0'
	`); err != nil {
		t.Fatalf("mark ephemeral child: %v", err)
	}

	// Automation-only: one non-archived, non-ephemeral child, but
	// automation-origin.
	insertTask(t, repo, ctx, "parent-automation-only", "ws-1", "Parent", "", "")
	insertTask(t, repo, ctx, "parent-automation-only-child-0", "ws-1", "Child", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET parent_id = 'parent-automation-only', state = 'COMPLETED', origin = 'automation_run'
		WHERE id = 'parent-automation-only-child-0'
	`); err != nil {
		t.Fatalf("mark automation-origin child: %v", err)
	}

	// Mixed non-member-only: one archived child, one ephemeral child, one
	// automation-origin child — still no wave member among them.
	insertTask(t, repo, ctx, "parent-mixed-non-member", "ws-1", "Parent", "", "")
	insertTask(t, repo, ctx, "parent-mixed-non-member-child-archived", "ws-1", "Child", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET parent_id = 'parent-mixed-non-member', state = 'COMPLETED', archived_at = datetime('now')
		WHERE id = 'parent-mixed-non-member-child-archived'
	`); err != nil {
		t.Fatalf("archive mixed child: %v", err)
	}
	insertTask(t, repo, ctx, "parent-mixed-non-member-child-ephemeral", "ws-1", "Child", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET parent_id = 'parent-mixed-non-member', state = 'COMPLETED', is_ephemeral = 1
		WHERE id = 'parent-mixed-non-member-child-ephemeral'
	`); err != nil {
		t.Fatalf("mark mixed ephemeral child: %v", err)
	}
	insertTask(t, repo, ctx, "parent-mixed-non-member-child-automation", "ws-1", "Child", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET parent_id = 'parent-mixed-non-member', state = 'COMPLETED', origin = 'automation_run'
		WHERE id = 'parent-mixed-non-member-child-automation'
	`); err != nil {
		t.Fatalf("mark mixed automation child: %v", err)
	}

	for _, id := range []string{"parent-ephemeral-only", "parent-automation-only", "parent-mixed-non-member"} {
		if _, err := repo.ExecRaw(ctx,
			`UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, id,
		); err != nil {
			t.Fatalf("mark %s as Office task: %v", id, err)
		}
		seedWakeAgentProfile(t, repo, ctx, id+"-agent", "idle")
		seedRunner(t, repo, ctx, id)
	}

	// Control: a genuine Office candidate with a real wave member.
	seedWakeCandidate(t, repo, ctx, "ws-1", "parent-normal")

	candidates, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ParentTaskID != "parent-normal" {
		t.Fatalf("candidates = %#v, want exactly [parent-normal] (no other parent has a wave member)", candidates)
	}
}

// TestListStuckParents_KeyedRunRetiresTimestampFallbackForParent is
// AC-OFFICE-WAKE-WAVE-IDENTITY-003.6's regression test: the pre-upgrade
// timestamp-comparison compatibility path is evaluated once per parent, not
// once per row. Once any wave-keyed run exists for a parent (in any
// status, for any wave), the timestamp rule is retired for that parent
// forever — even an older pre-upgrade row on the same parent, whose
// requested_at is recent enough it would otherwise still be blocking under
// the timestamp rule, no longer suppresses candidacy.
func TestListStuckParents_KeyedRunRetiresTimestampFallbackForParent(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	const (
		parentID = "parent-1"
		wsID     = "ws-1"
		oldTime  = "2026-01-01 00:00:00"
		newTime  = "2026-01-01 00:10:00"
	)

	insertTaskAt(t, repo, ctx, parentID, wsID, oldTime)
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, parentID,
	); err != nil {
		t.Fatalf("mark parent as Office task: %v", err)
	}
	child0 := parentID + "-child-0"
	insertTaskAt(t, repo, ctx, child0, wsID, newTime)
	setChildStateAt(t, repo, ctx, parentID, child0, "COMPLETED", newTime)
	seedWakeAgentProfile(t, repo, ctx, parentID+"-agent", "idle")
	seedRunner(t, repo, ctx, parentID)

	// A pre-upgrade terminal run (no wave identity at all) requested after
	// the child completed: under the timestamp-only rule this alone would
	// still block the parent.
	seedWakeRunAt(t, repo, ctx, "run-pre-upgrade", parentID, "task_children_completed",
		"finished", "'2026-01-01 00:20:00'")

	// A wave-keyed run also exists for this parent, for an unrelated
	// (already-stale) wave — it must not itself block via the wave-string
	// comparison, only retire the timestamp fallback.
	staleWaveString := waveidentity.WaveString(parentID, []string{"some-other-child"})
	staleWaveKey := waveidentity.WaveKey(parentID, []string{"some-other-child"})
	seedWakeRunWithWave(t, repo, ctx, "run-keyed", parentID, "task_children_completed",
		"finished", "'2026-01-01 00:01:00'", staleWaveKey, staleWaveString)

	candidates, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ParentTaskID != parentID {
		t.Fatalf("candidates = %#v, want exactly [%s]: a keyed run must retire the timestamp "+
			"fallback for this parent even though a recent pre-upgrade terminal run exists", candidates, parentID)
	}
}
