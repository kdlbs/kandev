import { describe, expect, it } from "vitest";
import type { TaskMR } from "@/lib/types/gitlab";
import { deriveMRTaskStatusSummary } from "./mr-task-status-summary";

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "id",
    task_id: "task-1",
    host: "https://gitlab.com",
    project_path: "group/project",
    mr_iid: 12,
    mr_url: "",
    mr_title: "Fix the thing",
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

describe("deriveMRTaskStatusSummary", () => {
  it("derives review, CI, and merge rows for an open, approved, mergeable MR", () => {
    const mr = makeMR({
      approval_state: "approved",
      pipeline_state: "success",
      detailed_merge_status: "mergeable",
    });

    const summary = deriveMRTaskStatusSummary(mr, true);

    expect(summary).toEqual({
      iid: 12,
      title: "Fix the thing",
      rows: [
        { kind: "review", status: "approved", tone: "success" },
        { kind: "ci", status: "passed", tone: "success" },
        { kind: "merge", status: "ready", tone: "success" },
      ],
    });
  });

  it("renders CI failed in the danger tone", () => {
    const summary = deriveMRTaskStatusSummary(makeMR({ pipeline_state: "failure" }), false);
    expect(summary.rows).toContainEqual({ kind: "ci", status: "failed", tone: "danger" });
  });

  it("renders merge conflicts in the danger tone from detailed_merge_status", () => {
    const summary = deriveMRTaskStatusSummary(
      makeMR({ detailed_merge_status: "cannot_be_merged" }),
      false,
    );
    expect(summary.rows).toContainEqual({ kind: "merge", status: "conflicts", tone: "danger" });
  });

  it("renders merge conflicts from the legacy merge_status when detailed_merge_status is absent", () => {
    const summary = deriveMRTaskStatusSummary(makeMR({ merge_status: "cannot_be_merged" }), false);
    expect(summary.rows).toContainEqual({ kind: "merge", status: "conflicts", tone: "danger" });
  });

  it("renders a draft MR with the draft merge row", () => {
    const summary = deriveMRTaskStatusSummary(makeMR({ draft: true }), false);
    expect(summary.rows).toEqual([{ kind: "merge", status: "draft", tone: "muted" }]);
  });

  it("renders a State row and omits the Merge row for a merged MR", () => {
    const summary = deriveMRTaskStatusSummary(
      makeMR({
        state: "merged",
        pipeline_state: "success",
        detailed_merge_status: "mergeable",
      }),
      false,
    );
    expect(summary.rows).toEqual([
      { kind: "state", status: "merged", tone: "merged" },
      { kind: "ci", status: "passed", tone: "success" },
    ]);
  });

  it("renders a Closed state row for a closed MR", () => {
    const summary = deriveMRTaskStatusSummary(makeMR({ state: "closed" }), false);
    expect(summary.rows).toEqual([{ kind: "state", status: "closed", tone: "danger" }]);
  });

  it("renders a Closed state row for a locked MR", () => {
    const summary = deriveMRTaskStatusSummary(makeMR({ state: "locked" }), false);
    expect(summary.rows).toEqual([{ kind: "state", status: "closed", tone: "danger" }]);
  });
});

describe("deriveMRTaskStatusSummary — empty and unrecognized values", () => {
  it("renders no rows for an MR with every optional field empty", () => {
    const summary = deriveMRTaskStatusSummary(makeMR(), false);
    expect(summary.rows).toEqual([]);
    expect(summary.iid).toBe(12);
    expect(summary.title).toBe("Fix the thing");
  });

  it("preserves an unrecognized pipeline_state verbatim in a raw row", () => {
    const summary = deriveMRTaskStatusSummary(
      makeMR({ pipeline_state: "canceled" as unknown as TaskMR["pipeline_state"] }),
      false,
    );
    expect(summary.rows).toContainEqual({
      kind: "ci",
      status: "raw",
      tone: "muted",
      rawValue: "canceled",
    });
  });

  it("preserves an unrecognized detailed_merge_status verbatim in a raw row", () => {
    const summary = deriveMRTaskStatusSummary(
      makeMR({ detailed_merge_status: "policies_denied" }),
      false,
    );
    expect(summary.rows).toContainEqual({
      kind: "merge",
      status: "raw",
      tone: "muted",
      rawValue: "policies_denied",
    });
  });

  it("omits the merge row entirely for checking/unchecked/ci_still_running (the CI row already says it)", () => {
    for (const status of ["checking", "unchecked", "ci_still_running"]) {
      const summary = deriveMRTaskStatusSummary(makeMR({ detailed_merge_status: status }), false);
      expect(summary.rows.find((row) => row.kind === "merge")).toBeUndefined();
    }
  });

  it("renders an awaiting-approval review row in the info tone", () => {
    const summary = deriveMRTaskStatusSummary(makeMR({ approval_state: "pending" }), false);
    expect(summary.rows).toContainEqual({
      kind: "review",
      status: "awaiting_approval",
      tone: "info",
    });
  });

  it("does not render a merge row for a non-open MR even when readyToMerge is true", () => {
    const summary = deriveMRTaskStatusSummary(makeMR({ state: "closed" }), true);
    expect(summary.rows.find((row) => row.kind === "merge")).toBeUndefined();
  });
});
