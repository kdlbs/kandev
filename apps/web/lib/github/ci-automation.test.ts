import { describe, expect, it } from "vitest";
import {
  autoFixRoundForState,
  clampAutoFixRound,
  deriveCIAutomationQueueState,
  findCIAutomationStateForPR,
  findPRAutomationOptionsForPR,
  normalizeAutoFixMaxRounds,
} from "./ci-automation";
import type { TaskCIPRAutomationState, TaskPR, TaskPRAutomationOptions } from "@/lib/types/github";

function makePR(repositoryID: string, prNumber = 42, taskID = "task-1"): TaskPR {
  return {
    task_id: taskID,
    repository_id: repositoryID,
    pr_number: prNumber,
  } as TaskPR;
}

function makePRAutomationOptions(
  repositoryID: string,
  prNumber: number,
  overrides: Partial<TaskPRAutomationOptions> = {},
): TaskPRAutomationOptions {
  return {
    task_id: "task-1",
    repository_id: repositoryID,
    pr_number: prNumber,
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    prompt_on_review_requested: false,
    prompt_on_merged: false,
    prompt_on_closed: false,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function makeState(
  repositoryID: string,
  prNumber: number,
  roundCount: number,
  exhaustedAt: string | null = null,
): TaskCIPRAutomationState {
  return {
    repository_id: repositoryID,
    pr_number: prNumber,
    auto_fix_round_count: roundCount,
    auto_fix_exhausted_at: exhaustedAt,
  } as TaskCIPRAutomationState;
}

describe("CI automation helpers", () => {
  it("finds PR automation state by repository id and PR number", () => {
    const states = [makeState("", 42, 1), makeState("repo-1", 42, 2), makeState("repo-1", 7, 3)];

    expect(findCIAutomationStateForPR(states, makePR("repo-1", 42))).toBe(states[1]);
    expect(findCIAutomationStateForPR(states, makePR("", 42))).toBe(states[0]);
    expect(findCIAutomationStateForPR(states, makePR("repo-missing", 42))).toBeUndefined();
  });

  it("returns an empty round state when no PR state exists", () => {
    expect(autoFixRoundForState(undefined, 12)).toEqual({
      current: 0,
      max: 12,
      exhausted: false,
    });
  });

  it("treats only backend exhaustion timestamps as paused", () => {
    expect(autoFixRoundForState(makeState("repo-1", 42, 10), 10)).toEqual({
      current: 10,
      max: 10,
      exhausted: false,
    });
    expect(autoFixRoundForState(makeState("repo-1", 42, 3, "2026-06-18T11:00:00Z"), 10)).toEqual({
      current: 3,
      max: 10,
      exhausted: true,
    });
  });

  it("finds PR automation options by repository id and PR number, independent of other PRs", () => {
    const options = [
      makePRAutomationOptions("repo-1", 1, { auto_fix_enabled: true }),
      makePRAutomationOptions("repo-1", 2, { auto_fix_enabled: false }),
      makePRAutomationOptions("repo-2", 1, { auto_merge_enabled: true }),
    ];

    expect(findPRAutomationOptionsForPR(options, makePR("repo-1", 1))).toBe(options[0]);
    expect(findPRAutomationOptionsForPR(options, makePR("repo-1", 2))).toBe(options[1]);
    // Same PR number, different repository — must not collide with repo-1's entry.
    expect(findPRAutomationOptionsForPR(options, makePR("repo-2", 1))).toBe(options[2]);
  });

  it("returns all-off defaults for a PR with no stored automation row", () => {
    const options = [makePRAutomationOptions("repo-1", 1, { auto_fix_enabled: true })];

    const result = findPRAutomationOptionsForPR(options, makePR("repo-1", 99, "task-9"));
    expect(result).toEqual({
      task_id: "task-9",
      repository_id: "repo-1",
      pr_number: 99,
      auto_fix_enabled: false,
      auto_merge_enabled: false,
      prompt_on_review_requested: false,
      prompt_on_merged: false,
      prompt_on_closed: false,
      created_at: "",
      updated_at: "",
    });
  });

  it("returns all-off defaults when pr_options is undefined", () => {
    expect(findPRAutomationOptionsForPR(undefined, makePR("repo-1", 1)).auto_fix_enabled).toBe(
      false,
    );
  });

  it("normalizes max rounds and clamps current rounds", () => {
    expect(normalizeAutoFixMaxRounds(undefined)).toBe(10);
    expect(normalizeAutoFixMaxRounds(null)).toBe(10);
    expect(normalizeAutoFixMaxRounds(Number.NaN)).toBe(10);
    expect(normalizeAutoFixMaxRounds(0)).toBe(1);
    expect(normalizeAutoFixMaxRounds(-4)).toBe(1);
    expect(normalizeAutoFixMaxRounds(12.8)).toBe(12);

    expect(clampAutoFixRound(undefined, 10)).toBe(0);
    expect(clampAutoFixRound(Number.NaN, 10)).toBe(0);
    expect(clampAutoFixRound(-2, 10)).toBe(0);
    expect(clampAutoFixRound(4.9, 10)).toBe(4);
    expect(clampAutoFixRound(14, 10)).toBe(10);
  });
});

describe("CI merge queue recovery helpers", () => {
  function makeQueuePR(overrides: Partial<TaskPR> = {}): TaskPR {
    return { ...makePR("", 42), ...overrides };
  }

  it("prioritizes an active queue entry", () => {
    const result = deriveCIAutomationQueueState(
      makeQueuePR({
        state: "open",
        merge_queue_state: "queued",
        merge_queue_entry_id: "entry-a",
        head_sha: "head-a",
      }),
      makePRAutomationOptions("repo-1", 42),
      undefined,
    );

    expect(result.status).toBe("queued");
    expect(result.context).toBe("queued");
  });

  it("shows an actionable removal until one repair round is accepted", () => {
    const pr = makeQueuePR({
      pr_number: 42,
      head_sha: "head-a",
      merge_queue_last_removal_id: "removal-a",
    });
    const options = makePRAutomationOptions("", 42);
    const result = deriveCIAutomationQueueState(pr, options, {
      last_queue_removal_cause: "checks_failed",
    } as TaskCIPRAutomationState);

    expect(result.status).toBe("removed_actionable");
    expect(result.removalCause).toBe("checks_failed");
    expect(result.repairAccepted).toBe(false);
  });

  it("uses generic copy state for unknown removals", () => {
    const result = deriveCIAutomationQueueState(
      makeQueuePR({ pr_number: 42, head_sha: "head-a", merge_queue_last_removal_id: "removal-a" }),
      makePRAutomationOptions("", 42),
      { last_queue_removal_cause: "provider_changed" } as TaskCIPRAutomationState,
    );

    expect(result.status).toBe("removed_not_actionable");
    expect(result.removalCause).toBe("unknown");
  });

  it("classifies the persisted removal reason when CI state is absent", () => {
    const result = deriveCIAutomationQueueState(
      makeQueuePR({
        pr_number: 42,
        head_sha: "head-a",
        merge_queue_last_removal_id: "removal-a",
        merge_queue_last_removal_reason: "CI checks failed on merge group",
      }),
      makePRAutomationOptions("", 42),
      undefined,
    );

    expect(result.status).toBe("removed_actionable");
    expect(result.removalCause).toBe("checks_failed");
  });

  it("distinguishes accepted repair, same-head wait, and pending new-head checks", () => {
    const pr = makeQueuePR({
      pr_number: 42,
      head_sha: "head-a",
      merge_queue_last_removal_id: "removal-a",
    });
    const options = makePRAutomationOptions("", 42, { auto_merge_enabled: true });
    const accepted = deriveCIAutomationQueueState(pr, options, {
      last_queue_attempt_head_sha: "head-a",
      last_queue_fix_event_id: "removal-a",
      last_queue_removal_cause: "checks_failed",
    } as TaskCIPRAutomationState);

    expect(accepted.status).toBe("repair_requested");
    expect(accepted.waitingForCommit).toBe(true);

    const pending = deriveCIAutomationQueueState(
      { ...pr, head_sha: "head-b", checks_state: "pending" },
      options,
      {
        last_queue_attempt_head_sha: "head-a",
        last_queue_removal_cause: "checks_failed",
      } as TaskCIPRAutomationState,
    );
    expect(pending.status).toBe("waiting_for_checks");
  });
});
