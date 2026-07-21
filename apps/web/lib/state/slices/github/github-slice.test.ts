import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createGitHubSlice } from "./github-slice";
import type { GitHubSlice } from "./types";
import type {
  GitHubStatus,
  TaskCIAutomationOptions,
  TaskIssueLink,
  TaskPR,
} from "@/lib/types/github";

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

function makeCIOptions(overrides: Partial<TaskCIAutomationOptions> = {}): TaskCIAutomationOptions {
  return {
    task_id: "task-1",
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    auto_fix_prompt_override: null,
    effective_auto_fix_prompt: "Default CI prompt",
    using_default_prompt: true,
    updated_at: "2026-06-18T10:00:00Z",
    pr_states: [],
    ...overrides,
  };
}

const FUTURE_RESET = "2030-01-01T00:00:00Z";
const NOW = "2026-05-04T12:00:00Z";

const baseStatus: GitHubStatus = {
  authenticated: true,
  username: "octocat",
  auth_method: "pat",
  token_configured: true,
  required_scopes: ["repo"],
};

describe("applyGitHubRateLimitUpdate", () => {
  it("merges incoming snapshots into the existing status", () => {
    const store = makeStore();
    store.getState().setGitHubStatus({ ...baseStatus });

    store.getState().applyGitHubRateLimitUpdate({
      trigger: "graphql",
      snapshots: [
        {
          resource: "graphql",
          remaining: 0,
          limit: 5000,
          reset_at: FUTURE_RESET,
          updated_at: NOW,
        },
        {
          resource: "core",
          remaining: 4500,
          limit: 5000,
          reset_at: FUTURE_RESET,
          updated_at: NOW,
        },
      ],
    });

    const status = store.getState().githubStatus.status;
    expect(status?.rate_limit?.graphql?.remaining).toBe(0);
    expect(status?.rate_limit?.graphql?.limit).toBe(5000);
    expect(status?.rate_limit?.core?.remaining).toBe(4500);
  });

  it("overwrites only the resources present in the update", () => {
    const store = makeStore();
    store.getState().setGitHubStatus({
      ...baseStatus,
      rate_limit: {
        core: {
          resource: "core",
          remaining: 4500,
          limit: 5000,
          reset_at: FUTURE_RESET,
          updated_at: NOW,
        },
      },
    });

    store.getState().applyGitHubRateLimitUpdate({
      trigger: "graphql",
      snapshots: [
        {
          resource: "graphql",
          remaining: 100,
          limit: 5000,
          reset_at: FUTURE_RESET,
          updated_at: NOW,
        },
      ],
    });

    const rl = store.getState().githubStatus.status?.rate_limit;
    expect(rl?.core?.remaining).toBe(4500); // untouched
    expect(rl?.graphql?.remaining).toBe(100);
  });

  it("is a no-op when status has not been hydrated yet", () => {
    const store = makeStore();
    expect(store.getState().githubStatus.status).toBeNull();

    store.getState().applyGitHubRateLimitUpdate({
      trigger: "core",
      snapshots: [
        {
          resource: "core",
          remaining: 0,
          limit: 5000,
          reset_at: FUTURE_RESET,
          updated_at: NOW,
        },
      ],
    });

    expect(store.getState().githubStatus.status).toBeNull();
  });
});

describe("setTaskPR", () => {
  it("appends a PR when the task has no rows yet", () => {
    const store = makeStore();
    const pr = makePR({ repository_id: "repo-a" });

    store.getState().setTaskPR("task-1", pr);

    expect(store.getState().taskPRs.byTaskId["task-1"]).toEqual([pr]);
  });

  it("upserts by repository_id so multi-repo PRs coexist", () => {
    const store = makeStore();
    const prA = makePR({ id: "a", repository_id: "repo-a", pr_number: 1 });
    const prB = makePR({ id: "b", repository_id: "repo-b", pr_number: 2 });
    const prAUpdated = makePR({ id: "a", repository_id: "repo-a", pr_number: 1, additions: 10 });

    store.getState().setTaskPR("task-1", prA);
    store.getState().setTaskPR("task-1", prB);
    store.getState().setTaskPR("task-1", prAUpdated);

    const list = store.getState().taskPRs.byTaskId["task-1"];
    expect(list).toHaveLength(2);
    expect(list.find((p) => p.repository_id === "repo-a")?.additions).toBe(10);
    expect(list.find((p) => p.repository_id === "repo-b")?.id).toBe("b");
  });

  it("keeps multi-branch PRs as siblings (same repo, different pr_number)", () => {
    const store = makeStore();
    const pr1 = makePR({ id: "p1", repository_id: "repo-a", pr_number: 1221 });
    const pr2 = makePR({ id: "p2", repository_id: "repo-a", pr_number: 1222 });
    const pr1Updated = makePR({
      id: "p1",
      repository_id: "repo-a",
      pr_number: 1221,
      additions: 99,
    });

    store.getState().setTaskPR("task-1", pr1);
    store.getState().setTaskPR("task-1", pr2);
    store.getState().setTaskPR("task-1", pr1Updated);

    const list = store.getState().taskPRs.byTaskId["task-1"];
    expect(list).toHaveLength(2);
    expect(list.find((p) => p.pr_number === 1221)?.additions).toBe(99);
    expect(list.find((p) => p.pr_number === 1222)?.id).toBe("p2");
  });

  it("heals a corrupted non-array entry instead of throwing", () => {
    // Simulates a stray payload landing in byTaskId[taskId] as something other
    // than an array (e.g. a partial hydration). The next setTaskPR call must
    // recover rather than propagate the bad shape, otherwise downstream
    // renderers crash with `prs is not iterable`.
    const store = makeStore();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    store.getState().setTaskPRs({ "task-1": {} as any });

    const pr = makePR({ repository_id: "repo-a" });
    expect(() => store.getState().setTaskPR("task-1", pr)).not.toThrow();

    expect(Array.isArray(store.getState().taskPRs.byTaskId["task-1"])).toBe(true);
    expect(store.getState().taskPRs.byTaskId["task-1"]).toEqual([pr]);
  });
});

describe("setTaskIssues", () => {
  const link: TaskIssueLink = {
    task_id: "task-1",
    task_title: "Fix issue",
    owner: "kdlbs",
    repo: "kandev",
    issue_number: 1672,
    issue_url: "https://github.com/kdlbs/kandev/issues/1672",
    issue_title: "Issue title",
  };

  it("replaces workspace issue links by task id", () => {
    const store = makeStore();

    store.getState().setTaskIssues("ws-1", { "task-1": link });

    expect(store.getState().taskIssues).toEqual({
      workspaceId: "ws-1",
      byTaskId: { "task-1": link },
    });
  });

  it("upserts issue links only for the active workspace", () => {
    const store = makeStore();
    store.getState().setTaskIssues("ws-1", {});

    store.getState().upsertTaskIssue("ws-2", link);
    expect(store.getState().taskIssues.byTaskId).toEqual({});

    store.getState().upsertTaskIssue("ws-1", link);
    expect(store.getState().taskIssues.byTaskId).toEqual({ "task-1": link });
  });

  it("initializes an unloaded workspace with a newly linked issue", () => {
    const store = makeStore();

    store.getState().upsertTaskIssue("ws-1", link);

    expect(store.getState().taskIssues).toEqual({
      workspaceId: "ws-1",
      byTaskId: { "task-1": link },
    });
  });
});

describe("setPendingPrUrlForTask", () => {
  it("stores a pending PR URL until TaskPR sync clears it", () => {
    const store = makeStore();

    store
      .getState()
      .setPendingPrUrlForTask("task-1", "", "https://dev.azure.com/o/p/_git/r/pullrequest/1");
    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]?.[""]).toBe(
      "https://dev.azure.com/o/p/_git/r/pullrequest/1",
    );

    store.getState().setTaskPR("task-1", makePR());
    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]).toBeUndefined();
  });

  it("clears only the synced repo pending URL in multi-repo tasks", () => {
    const store = makeStore();
    const urlA = "https://dev.azure.com/o/p/_git/a/pullrequest/1";
    const urlB = "https://dev.azure.com/o/p/_git/b/pullrequest/2";

    store.getState().setPendingPrUrlForTask("task-1", "repo-a", urlA);
    store.getState().setPendingPrUrlForTask("task-1", "repo-b", urlB);
    store.getState().setTaskPR("task-1", makePR({ repository_id: "repo-a", pr_url: urlA }));

    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]?.["repo-b"]).toBe(urlB);
    expect(store.getState().pendingPrUrlByTaskId.byTaskId["task-1"]?.["repo-a"]).toBeUndefined();
  });
});

describe("task CI automation options", () => {
  it("stores task options and per-task loading/saving/error state", () => {
    const store = makeStore();
    const options = makeCIOptions({ auto_fix_enabled: true });

    store.getState().setTaskCIAutomationLoading("task-1", true);
    store.getState().setTaskCIAutomationSaving("task-1", true);
    store.getState().setTaskCIAutomationError("task-1", "failed");
    store.getState().setTaskCIAutomationOptions("task-1", options);

    const state = store.getState().taskCIAutomation;
    expect(state.loading["task-1"]).toBe(true);
    expect(state.saving["task-1"]).toBe(true);
    expect(state.errors["task-1"]).toBe("failed");
    expect(state.byTaskId["task-1"]).toEqual(options);
  });
});
