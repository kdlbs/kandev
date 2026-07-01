import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createGitHubSlice } from "./github-slice";
import type { GitHubSlice } from "./types";
import type { PRFeedback } from "@/lib/types/github";

function makeStore() {
  return create<GitHubSlice>()(immer((...a) => createGitHubSlice(...a)));
}

describe("GitHub local pending PR URLs", () => {
  it("stores and clears pending URLs by task and repo key", () => {
    const store = makeStore();

    store.getState().setPendingPrUrlForTask("task-1", "repo-a", " https://example/pr/1 ");
    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]?.["repo-a"]).toBe(
      "https://example/pr/1",
    );

    store.getState().setPendingPrUrlForTask("task-1", "repo-a", "");
    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]).toBeUndefined();
  });

  it("clears only the synced PR pending URL", () => {
    const store = makeStore();
    const urlA = "https://example/pr/1";
    const urlB = "https://example/pr/2";

    store.getState().setPendingPrUrlForTask("task-1", "repo-a", urlA);
    store.getState().setPendingPrUrlForTask("task-1", "repo-b", urlB);
    store
      .getState()
      .clearPendingPrUrlForTaskPR("task-1", { repository_id: "repo-a", pr_url: urlA });

    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]?.["repo-a"]).toBeUndefined();
    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]?.["repo-b"]).toBe(urlB);
  });
});

describe("GitHub PR feedback cache", () => {
  it("bounds cached feedback entries", () => {
    const store = makeStore();

    for (let i = 0; i < 25; i++) {
      store
        .getState()
        .setPRFeedbackCacheEntry(`o/r#${i}`, { pr: { pr_number: i } } as unknown as PRFeedback);
    }

    const keys = Object.keys(store.getState().prFeedbackCache.byKey);
    expect(keys).toHaveLength(20);
    expect(keys).not.toContain("o/r#0");
    expect(keys).toContain("o/r#24");
  });
});
