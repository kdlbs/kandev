/* eslint-disable sonarjs/no-duplicate-string */
import { describe, expect, it } from "vitest";
import { officeTaskFiltersKey, qk, taskListFiltersKey } from "./keys";

describe("query keys", () => {
  it("normalizes optional task list filters into serializable values", () => {
    expect(taskListFiltersKey({ workflowId: undefined, repositoryId: null })).toEqual({
      page: null,
      pageSize: null,
      query: "",
      includeArchived: false,
      workflowId: null,
      repositoryId: null,
      sort: null,
    });
    expect(() =>
      JSON.stringify(qk.tasks.infinite("workspace-1", { query: "search" })),
    ).not.toThrow();
  });

  it("sorts array filters so equivalent office task filters produce the same key", () => {
    expect(
      officeTaskFiltersKey({
        status: ["done", "todo"],
        priority: ["p2", "p1"],
        assignee: ["agent-2", "agent-1"],
        project: ["project-2", "project-1"],
      }),
    ).toEqual(
      officeTaskFiltersKey({
        status: ["todo", "done"],
        priority: ["p1", "p2"],
        assignee: ["agent-1", "agent-2"],
        project: ["project-1", "project-2"],
      }),
    );
  });

  it("keeps pagination cursors out of infinite query keys", () => {
    expect(qk.session.messagesInfinite("session-1", { limit: 25 })).toEqual(
      qk.session.messagesInfinite("session-1", { limit: 25 }),
    );
    expect(qk.office.tasks("workspace-1", { limit: 25 })).toEqual(
      qk.office.tasks("workspace-1", { limit: 25 }),
    );
  });

  it("keeps workflow step keys scoped by workflow", () => {
    expect(qk.workflows.steps("workflow-1")).toEqual(["workflows", "workflow-1", "steps"]);
    expect(() => JSON.stringify(qk.workflows.steps("workflow-1"))).not.toThrow();
  });

  it("keeps session plan and queue keys stable and serializable", () => {
    expect(qk.taskPlan.detail("task-1")).toEqual(["taskPlan", "task-1"]);
    expect(qk.taskPlan.revisions("task-1")).toEqual(["taskPlan", "task-1", "revisions"]);
    expect(qk.taskPlan.revision("task-1", "revision-1")).toEqual([
      "taskPlan",
      "task-1",
      "revisions",
      "revision-1",
    ]);
    expect(qk.session.queue("session-1")).toEqual(["session", "session-1", "queue"]);
    expect(() => JSON.stringify(qk.taskPlan.revision("task-1", "revision-1"))).not.toThrow();
  });

  it("keeps session runtime keys scoped and serializable", () => {
    expect(qk.sessionRuntime.gitStatus("env-1")).toEqual([
      "sessionRuntime",
      "environment",
      "env-1",
      "gitStatus",
    ]);
    expect(qk.sessionRuntime.userShells("env-1", "task-1")).toEqual([
      "sessionRuntime",
      "environment",
      "env-1",
      "userShells",
      { taskId: "task-1" },
    ]);
    expect(qk.sessionRuntime.models("session-1")).toEqual([
      "sessionRuntime",
      "session",
      "session-1",
      "models",
    ]);
    expect(() => JSON.stringify(qk.sessionRuntime.agentctl("session-1"))).not.toThrow();
  });

  it("keeps office task detail subresources on stable serializable keys", () => {
    expect(qk.office.taskComments("task-1")).toEqual(["office", "tasks", "task-1", "comments"]);
    expect(qk.office.taskActivity("workspace-1", "task-1")).toEqual([
      "office",
      "workspaces",
      "workspace-1",
      "tasks",
      "task-1",
      "activity",
    ]);
    expect(() => JSON.stringify(qk.office.taskSearch("workspace-1", "needle", 10))).not.toThrow();
  });
});
