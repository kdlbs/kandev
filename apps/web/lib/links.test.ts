import { afterEach, describe, expect, it, vi } from "vitest";
import {
  isTaskDetailPath,
  linkToTask,
  linkToTaskOverview,
  linkToTasks,
  linkToThreads,
  normalizePathname,
  replaceTaskUrl,
} from "./links";

describe("task links", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("uses /t as the canonical task detail route", () => {
    expect(linkToTask("task-123")).toBe("/t/task-123");
    expect(linkToTask("task-123", "plan")).toBe("/t/task-123?layout=plan");
  });

  it("keeps /tasks for the task list route", () => {
    expect(linkToTasks()).toBe("/tasks");
    expect(linkToTasks("workspace-123")).toBe("/tasks?workspace=workspace-123");
  });

  it("marks deliberate overview navigation so startup routing does not resume a task", () => {
    expect(linkToTaskOverview()).toBe("/?home=overview");
    expect(linkToTaskOverview({ workspaceId: "workspace 123" })).toBe(
      "/?home=overview&workspaceId=workspace+123",
    );
    expect(linkToTaskOverview({ workflowId: "workflow-123" })).toBe(
      "/?home=overview&workflowId=workflow-123",
    );
  });

  it("replaces the browser URL with the canonical task detail route", () => {
    const replaceState = vi.spyOn(window.history, "replaceState").mockImplementation(() => {});

    replaceTaskUrl("task-123");

    expect(replaceState).toHaveBeenCalledWith({}, "", "/t/task-123");
  });
});

describe("Threads links", () => {
  it("carries the task and selected session for a Threads deep link", () => {
    expect(linkToThreads("workspace-123", "task-123", "session-123")).toBe(
      "/threads?workspace=workspace-123&taskId=task-123&sessionId=session-123",
    );
  });

  it("does not emit a session parameter without a task", () => {
    expect(linkToThreads("workspace-123", undefined, "session-123")).toBe(
      "/threads?workspace=workspace-123",
    );
  });
});

describe("isTaskDetailPath", () => {
  it("matches the canonical and compatibility detail routes", () => {
    expect(isTaskDetailPath("/t/task-123", "task-123")).toBe(true);
    expect(isTaskDetailPath("/tasks/task-123", "task-123")).toBe(true);
  });

  it("matches trailing-slash variants the SPA normalizes", () => {
    expect(isTaskDetailPath("/t/task-123/", "task-123")).toBe(true);
    expect(isTaskDetailPath("/tasks/task-123/", "task-123")).toBe(true);
  });

  it("does not match a different task id", () => {
    expect(isTaskDetailPath("/t/other", "task-123")).toBe(false);
    expect(isTaskDetailPath("/tasks/other", "task-123")).toBe(false);
  });

  it("does not match the task list route or unrelated paths", () => {
    expect(isTaskDetailPath("/tasks", "task-123")).toBe(false);
    expect(isTaskDetailPath("/", "task-123")).toBe(false);
    expect(isTaskDetailPath("/t/task-123/extra", "task-123")).toBe(false);
  });
});

describe("normalizePathname", () => {
  it("removes one trailing slash except from the root path", () => {
    expect(normalizePathname("/")).toBe("/");
    expect(normalizePathname("/office/tasks/task-123/")).toBe("/office/tasks/task-123");
    expect(normalizePathname("/office/tasks/task-123")).toBe("/office/tasks/task-123");
  });
});
