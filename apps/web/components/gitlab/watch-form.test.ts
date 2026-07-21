import { describe, expect, it } from "vitest";
import { buildWatchPayload, makeWatchForm, watchFormFromWatch } from "./watch-form";

describe("GitLab watch form", () => {
  it("leaves the default review query empty so the backend constrains by reviewer", () => {
    const form = makeWatchForm("review", "ws-1");
    expect(form.customQuery).toBe("");
    expect(form.prompt).toContain("{{mr.url}}");
  });

  it("leaves the default issue query empty so the backend constrains by assignee", () => {
    const form = makeWatchForm("issue", "ws-1");
    expect(form.customQuery).toBe("");
    expect(form.prompt).toContain("{{issue.url}}");
  });

  it("builds the full review watch payload", () => {
    const form = {
      ...makeWatchForm("review", "ws-1"),
      workflowId: "workflow",
      workflowStepId: "step",
      projectPaths: "group/api, group/web",
      repositoryId: "repo-1",
      baseBranch: "develop",
      prompt: "Review {{mr_url}}",
      maxInflightTasks: "4",
    };

    expect(buildWatchPayload("review", form)).toMatchObject({
      workspace_id: "ws-1",
      projects: [{ path: "group/api" }, { path: "group/web" }],
      repository_id: "repo-1",
      base_branch: "develop",
      max_inflight_tasks: 4,
      review_scope: "user",
    });
  });

  it("round trips issue labels and leaves an empty inflight limit unset", () => {
    const form = watchFormFromWatch("issue", {
      id: "watch-1",
      workspace_id: "ws-1",
      workflow_id: "workflow",
      workflow_step_id: "step",
      projects: [],
      agent_profile_id: "",
      executor_profile_id: "",
      prompt: "Fix {{issue_url}}",
      labels: ["bug", "priority::high"],
      custom_query: "state=opened",
      enabled: true,
      poll_interval_seconds: 300,
      cleanup_policy: "auto",
      created_at: "2026-01-01",
      updated_at: "2026-01-01",
    });

    expect(buildWatchPayload("issue", form)).toMatchObject({
      labels: ["bug", "priority::high"],
      max_inflight_tasks: undefined,
    });
  });

  it("rejects missing workflow dependencies and invalid numeric limits", () => {
    const form = { ...makeWatchForm("review", "ws-1"), maxInflightTasks: "0" };
    expect(buildWatchPayload("review", form)).toBeNull();
  });

  it("sends zero to clear an empty in-flight cap while editing", () => {
    const form = {
      ...makeWatchForm("issue", "ws-1"),
      workflowId: "workflow",
      workflowStepId: "step",
      maxInflightTasks: "",
    };

    expect(buildWatchPayload("issue", form, true)?.max_inflight_tasks).toBe(0);
    expect(buildWatchPayload("issue", form)?.max_inflight_tasks).toBeUndefined();
  });
});
