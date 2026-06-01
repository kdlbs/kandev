import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createGitHubSlice } from "./github-slice";
import type { GitHubSlice } from "./types";
import type { TaskPR } from "@/lib/types/github";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    task_id: "task-1",
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
    ...overrides,
  };
}

function makeStore() {
  return create<GitHubSlice>()(immer((...a) => createGitHubSlice(...a)));
}

describe("setPendingPrUrlForTask", () => {
  it("stores a pending PR URL", () => {
    const store = makeStore();
    const url = "https://dev.azure.com/o/p/_git/r/pullrequest/1";

    store.getState().setPendingPrUrlForTask("task-1", "", url);

    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]?.[""]).toBe(url);
  });

  it("clears the pending URL when an empty value is set", () => {
    const store = makeStore();
    store.getState().setPendingPrUrlForTask("task-1", "", "https://example.com/pr/1");

    store.getState().setPendingPrUrlForTask("task-1", "", "   ");

    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]).toBeUndefined();
  });
});

describe("reconcilePendingPrUrls", () => {
  it("clears the pending URL once the matching TaskPR lands", () => {
    const store = makeStore();
    store
      .getState()
      .setPendingPrUrlForTask("task-1", "", "https://dev.azure.com/o/p/_git/r/pullrequest/1");

    store.getState().reconcilePendingPrUrls("task-1", [makePR()]);

    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]).toBeUndefined();
  });

  it("clears only the synced repo pending URL in multi-repo tasks", () => {
    const store = makeStore();
    const urlA = "https://dev.azure.com/o/p/_git/a/pullrequest/1";
    const urlB = "https://dev.azure.com/o/p/_git/b/pullrequest/2";

    store.getState().setPendingPrUrlForTask("task-1", "repo-a", urlA);
    store.getState().setPendingPrUrlForTask("task-1", "repo-b", urlB);

    store
      .getState()
      .reconcilePendingPrUrls("task-1", [makePR({ repository_id: "repo-a", pr_url: urlA })]);

    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]?.["repo-b"]).toBe(urlB);
    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]?.["repo-a"]).toBeUndefined();
  });

  it("is a no-op when the task has no pending URLs", () => {
    const store = makeStore();
    expect(() => store.getState().reconcilePendingPrUrls("task-1", [makePR()])).not.toThrow();
    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]).toBeUndefined();
  });
});
