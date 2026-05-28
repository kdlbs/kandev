import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createGitLabSlice } from "./gitlab-slice";
import type { GitLabSlice } from "./types";
import type { TaskMR } from "@/lib/types/gitlab";

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "id",
    task_id: "task-1",
    host: "https://gitlab.com",
    project_path: "acme/api",
    mr_iid: 1,
    mr_url: "https://gitlab.com/acme/api/-/merge_requests/1",
    mr_title: "Test MR",
    head_branch: "feat",
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
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function makeStore() {
  return create<GitLabSlice>()(immer((...a) => createGitLabSlice(...a)));
}

describe("setTaskMRs", () => {
  it("replaces byTaskId wholesale", () => {
    const store = makeStore();
    const mrA = makeMR({ id: "a" });
    store.getState().setTaskMRs({ "task-1": [mrA] });
    expect(store.getState().taskMRs.byTaskId["task-1"]).toEqual([mrA]);

    store.getState().setTaskMRs({ "task-2": [makeMR({ id: "b" })] });
    expect(store.getState().taskMRs.byTaskId).not.toHaveProperty("task-1");
    expect(store.getState().taskMRs.byTaskId["task-2"]).toHaveLength(1);
  });
});

describe("setTaskMR", () => {
  it("appends an MR when the task has no rows yet", () => {
    const store = makeStore();
    const mr = makeMR({ repository_id: "repo-a" });

    store.getState().setTaskMR("task-1", mr);

    expect(store.getState().taskMRs.byTaskId["task-1"]).toEqual([mr]);
  });

  it("upserts the same (repo, project, iid) key in place", () => {
    const store = makeStore();
    const original = makeMR({ id: "a", repository_id: "repo-a", mr_iid: 5 });
    const updated = makeMR({
      id: "a",
      repository_id: "repo-a",
      mr_iid: 5,
      mr_title: "renamed",
    });

    store.getState().setTaskMR("task-1", original);
    store.getState().setTaskMR("task-1", updated);

    const list = store.getState().taskMRs.byTaskId["task-1"];
    expect(list).toHaveLength(1);
    expect(list[0]!.mr_title).toBe("renamed");
  });

  it("keeps multi-repo MRs distinct on (repository_id, project_path, mr_iid)", () => {
    const store = makeStore();
    // Same project + iid, different repositories — must coexist.
    const repoA = makeMR({ id: "a", repository_id: "repo-a", mr_iid: 1 });
    const repoB = makeMR({ id: "b", repository_id: "repo-b", mr_iid: 1 });
    // Same repo, different project — must also coexist.
    const repoAOtherProject = makeMR({
      id: "c",
      repository_id: "repo-a",
      project_path: "acme/web",
      mr_iid: 1,
    });
    // Same repo + project, different iid — must coexist (multiple open MRs).
    const repoASecondMR = makeMR({ id: "d", repository_id: "repo-a", mr_iid: 99 });

    store.getState().setTaskMR("task-1", repoA);
    store.getState().setTaskMR("task-1", repoB);
    store.getState().setTaskMR("task-1", repoAOtherProject);
    store.getState().setTaskMR("task-1", repoASecondMR);

    const list = store.getState().taskMRs.byTaskId["task-1"];
    expect(list).toHaveLength(4);
    expect(list.map((m) => m.id).sort()).toEqual(["a", "b", "c", "d"]);
  });

  it("treats missing repository_id as the empty key (single-repo tasks)", () => {
    // A multi-repo MR with repository_id and a single-repo MR with the same
    // (project_path, mr_iid) but no repository_id must still coexist — the
    // empty repo key is its own slot.
    const store = makeStore();
    const noRepo = makeMR({ id: "x", mr_iid: 1 });
    const withRepo = makeMR({ id: "y", repository_id: "repo-a", mr_iid: 1 });

    store.getState().setTaskMR("task-1", noRepo);
    store.getState().setTaskMR("task-1", withRepo);

    const list = store.getState().taskMRs.byTaskId["task-1"];
    expect(list).toHaveLength(2);
    expect(list.map((m) => m.id).sort()).toEqual(["x", "y"]);
  });
});

describe("resetTaskMRs", () => {
  it("clears byTaskId so a workspace switch doesn't leak previous MRs", () => {
    const store = makeStore();
    store.getState().setTaskMR("task-1", makeMR({ id: "a" }));
    expect(store.getState().taskMRs.byTaskId["task-1"]).toHaveLength(1);

    store.getState().resetTaskMRs();

    expect(store.getState().taskMRs.byTaskId).toEqual({});
  });
});

describe("review watches", () => {
  it("set + add + update + remove round-trip", () => {
    const store = makeStore();
    const watch = {
      id: "w-1",
      workspace_id: "ws-1",
      workflow_id: "",
      workflow_step_id: "",
      projects: [{ path: "group/proj" }],
      agent_profile_id: "",
      executor_profile_id: "",
      prompt: "",
      review_scope: "user",
      custom_query: "",
      enabled: true,
      poll_interval_seconds: 300,
      cleanup_policy: "auto",
      created_at: "",
      updated_at: "",
    };
    store.getState().setGitLabReviewWatches([watch]);
    expect(store.getState().gitlabReviewWatches.items).toHaveLength(1);
    expect(store.getState().gitlabReviewWatches.loaded).toBe(true);

    const watch2 = { ...watch, id: "w-2" };
    store.getState().addGitLabReviewWatch(watch2);
    expect(store.getState().gitlabReviewWatches.items).toHaveLength(2);

    store.getState().updateGitLabReviewWatchInStore({ ...watch, prompt: "updated" });
    expect(store.getState().gitlabReviewWatches.items.find((w) => w.id === "w-1")?.prompt).toBe(
      "updated",
    );

    store.getState().removeGitLabReviewWatch("w-1");
    expect(store.getState().gitlabReviewWatches.items).toHaveLength(1);
    expect(store.getState().gitlabReviewWatches.items[0].id).toBe("w-2");
  });
});

describe("issue watches", () => {
  it("set + add + remove round-trip", () => {
    const store = makeStore();
    const watch = {
      id: "iw-1",
      workspace_id: "ws-1",
      workflow_id: "",
      workflow_step_id: "",
      projects: [],
      agent_profile_id: "",
      executor_profile_id: "",
      prompt: "",
      labels: ["bug"],
      custom_query: "",
      enabled: true,
      poll_interval_seconds: 300,
      cleanup_policy: "auto",
      created_at: "",
      updated_at: "",
    };
    store.getState().setGitLabIssueWatches([watch]);
    expect(store.getState().gitlabIssueWatches.items).toHaveLength(1);

    store.getState().removeGitLabIssueWatch("iw-1");
    expect(store.getState().gitlabIssueWatches.items).toHaveLength(0);
  });
});

describe("action presets + stats", () => {
  it("upserts presets by workspace and stores stats with loadedAt timestamp", () => {
    const store = makeStore();
    const presets = {
      workspace_id: "ws-1",
      mr: [{ id: "x", label: "x", hint: "", icon: "", prompt_template: "" }],
      issue: [],
    };
    store.getState().setGitLabActionPresets("ws-1", presets);
    expect(store.getState().gitlabActionPresets.byWorkspaceId["ws-1"]).toEqual(presets);

    store.getState().setGitLabStats({
      open_mrs: 5,
      mrs_awaiting_my_review: 2,
      open_issues_assigned_me: 3,
    });
    expect(store.getState().gitlabStats.data?.open_mrs).toBe(5);
    expect(store.getState().gitlabStats.loadedAt).not.toBeNull();
  });
});
