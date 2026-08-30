import { describe, expect, it } from "vitest";
import type { TaskPR } from "@/lib/types/github";
import { getPRStatusColor } from "./pr-task-icon";
import { derivePRTaskStatusSummary } from "./pr-task-status-summary";

function draftPR(): TaskPR {
  return {
    id: "id",
    workspace_id: "workspace-1",
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
    review_state: "approved",
    checks_state: "success",
    mergeable_state: "draft",
    review_count: 1,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 1,
    checks_passing: 1,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
  };
}

describe("draft PR task status", () => {
  it("stays muted even when checks pass", () => {
    expect(getPRStatusColor(draftPR())).toBe("text-muted-foreground");
  });

  it("identifies the draft without claiming it is ready to merge", () => {
    const summary = derivePRTaskStatusSummary(draftPR(), false);
    expect(summary.rows.at(-1)).toEqual({ kind: "merge", status: "draft", tone: "muted" });
    expect(summary.rows.some((row) => row.status === "ready")).toBe(false);
  });
});
