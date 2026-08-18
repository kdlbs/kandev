import { describe, expect, it } from "vitest";
import { chipAutomation, selectBadgeMR } from "./mr-status-chip-selection";
import type { TaskMR, TaskMRAutomationOptions, TaskMRLifecycleState } from "@/lib/types/gitlab";

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "association-1",
    task_id: "task-1",
    host: "https://gitlab.example",
    project_path: "group/project",
    mr_iid: 81,
    mr_url: "https://gitlab.example/group/project/-/merge_requests/81",
    mr_title: "Test MR",
    head_branch: "feature",
    base_branch: "main",
    author_username: "alice",
    state: "open",
    approval_state: "",
    pipeline_state: "",
    merge_status: "",
    draft: false,
    approval_count: 0,
    required_approvals: 0,
    pipeline_jobs_total: 0,
    pipeline_jobs_pass: 0,
    reviewer_count: 0,
    unapproved_reviewers: 0,
    unresolved_discussions: 0,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function makeState(overrides: Partial<TaskMRLifecycleState> = {}): TaskMRLifecycleState {
  return {
    task_id: "task-1",
    repository_id: "",
    project_path: "group/project",
    mr_iid: 81,
    review_request_initialized: false,
    last_review_requested: false,
    last_observed_state: "",
    last_lifecycle_event: "",
    last_fix_signature: "",
    last_fix_checkpoint_json: "",
    last_merge_signature: "",
    created_at: "",
    updated_at: "",
    auto_fix_round_count: 0,
    ...overrides,
  };
}

function makeAutomation(overrides: Partial<TaskMRAutomationOptions> = {}): TaskMRAutomationOptions {
  return {
    task_id: "task-1",
    auto_fix_enabled: true,
    auto_merge_enabled: false,
    auto_fix_max_rounds: 5,
    effective_auto_fix_prompt: "",
    using_default_prompt: true,
    prompt_on_review_requested: false,
    prompt_on_merged: false,
    prompt_on_closed: false,
    review_reviewer_username: "",
    updated_at: "",
    mr_states: [],
    ...overrides,
  };
}

// spec: Selection and ordering, "The auto-fix round shown on a multi-MR chip
// is the most attention-worthy round across the open MRs" — an exhausted
// round beats a non-exhausted one, then a higher `current` wins, then the
// same mr_iid/project_path/id order selectChipMR uses. Previously had zero
// coverage of any kind (Review round 1 finding #1, mutation-proven: with
// isBadgeMRBetter neutered to `return false`, the full suite stayed green).
describe("selectBadgeMR tiebreak", () => {
  it("prefers an exhausted round over a non-exhausted higher round (spec.md:1063-1065)", () => {
    const exhausted = makeMR({ id: "a", mr_iid: 3 });
    const notExhausted = makeMR({ id: "b", mr_iid: 9 });
    const options = makeAutomation({
      mr_states: [
        makeState({
          mr_iid: 3,
          auto_fix_round_count: 3,
          auto_fix_exhausted_at: "2026-08-11T00:00:00Z",
        }),
        makeState({ mr_iid: 9, auto_fix_round_count: 4 }),
      ],
    });

    // Both input orders, so a first-in-array bug can't hide behind a
    // coincidentally-correct fixture order.
    expect(selectBadgeMR([exhausted, notExhausted], options)?.id).toBe("a");
    expect(selectBadgeMR([notExhausted, exhausted], options)?.id).toBe("a");
  });

  it("prefers a higher current round when neither is exhausted", () => {
    const lowerRound = makeMR({ id: "a", mr_iid: 1 });
    const higherRound = makeMR({ id: "b", mr_iid: 2 });
    const options = makeAutomation({
      mr_states: [
        makeState({ mr_iid: 1, auto_fix_round_count: 1 }),
        makeState({ mr_iid: 2, auto_fix_round_count: 4 }),
      ],
    });

    expect(selectBadgeMR([lowerRound, higherRound], options)?.id).toBe("b");
    expect(selectBadgeMR([higherRound, lowerRound], options)?.id).toBe("b");
  });

  it("resolves a fully-tied round by mr_iid ascending, iid 7 beating iid 12 (spec.md:1066-1071)", () => {
    const higherIID = makeMR({ id: "a", mr_iid: 12 });
    const lowerIID = makeMR({ id: "b", mr_iid: 7 });
    const options = makeAutomation({
      mr_states: [
        makeState({ mr_iid: 12, auto_fix_round_count: 2 }),
        makeState({ mr_iid: 7, auto_fix_round_count: 2 }),
      ],
    });

    // For either input order, the winner is the lower mr_iid (7), not
    // whichever MR happens to come first in the array.
    expect(selectBadgeMR([higherIID, lowerIID], options)?.mr_iid).toBe(7);
    expect(selectBadgeMR([lowerIID, higherIID], options)?.mr_iid).toBe(7);
  });

  it("returns null when auto-fix is disabled", () => {
    const mr = makeMR();
    expect(selectBadgeMR([mr], makeAutomation({ auto_fix_enabled: false }))).toBeNull();
  });
});

// Integration-level confirmation that chipAutomation's returned round info
// actually comes from the badge-selected MR identified above.
describe("chipAutomation", () => {
  it("returns the badge-selected MR's own round info", () => {
    const exhausted = makeMR({ id: "a", mr_iid: 3 });
    const notExhausted = makeMR({ id: "b", mr_iid: 9 });
    const options = makeAutomation({
      mr_states: [
        makeState({
          mr_iid: 3,
          auto_fix_round_count: 3,
          auto_fix_exhausted_at: "2026-08-11T00:00:00Z",
        }),
        makeState({ mr_iid: 9, auto_fix_round_count: 4 }),
      ],
    });

    const result = chipAutomation(options, [exhausted, notExhausted]);

    expect(result.autoFixRound).toEqual({ current: 3, max: 5, exhausted: true });
  });
});
