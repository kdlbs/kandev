import { describe, expect, it } from "vitest";
import { getPRStatusColor, hasAnyPRMergeConflict, hasPRMergeConflict } from "./pr-task-icon";
import type { TaskPR } from "@/lib/types/github";

const MUTED_FOREGROUND = "text-muted-foreground";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    task_id: "task",
    owner: "o",
    repo: "r",
    pr_number: 1,
    pr_url: "",
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
    workspace_id: "workspace-1",
    ...overrides,
  };
}

describe("PR merge conflict warning", () => {
  it("keeps a confirmed conflict separate from draft and failed checks", () => {
    const pr = makePR({
      mergeable_state: "draft",
      checks_state: "failure",
      has_merge_conflicts: true,
    });
    expect(getPRStatusColor(pr)).toBe(MUTED_FOREGROUND);
    expect(hasPRMergeConflict(pr)).toBe(true);
  });

  it("uses legacy dirty only when no explicit observation exists", () => {
    expect(hasPRMergeConflict(makePR({ mergeable_state: "dirty" }))).toBe(true);
    expect(
      hasPRMergeConflict(makePR({ mergeable_state: "dirty", has_merge_conflicts: false })),
    ).toBe(false);
  });

  it("ignores terminal conflicts and highlights an open sibling", () => {
    expect(
      hasAnyPRMergeConflict([
        makePR({ state: "merged", has_merge_conflicts: true }),
        makePR({ pr_number: 2, has_merge_conflicts: false }),
      ]),
    ).toBe(false);
    expect(
      hasAnyPRMergeConflict([
        makePR({ state: "merged", has_merge_conflicts: true }),
        makePR({ pr_number: 2, has_merge_conflicts: true }),
      ]),
    ).toBe(true);
  });
});
