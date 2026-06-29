import { afterEach, describe, expect, it, vi } from "vitest";
import { isTaskDetailPath, linkToTask, linkToTasks, replaceTaskUrl } from "./links";

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

  it("replaces the browser URL with the canonical task detail route", () => {
    const replaceState = vi.spyOn(window.history, "replaceState").mockImplementation(() => {});

    replaceTaskUrl("task-123");

    expect(replaceState).toHaveBeenCalledWith({}, "", "/t/task-123");
  });
});

describe("isTaskDetailPath", () => {
  it("matches the canonical and compatibility detail routes", () => {
    expect(isTaskDetailPath("/t/task-123", "task-123")).toBe(true);
    expect(isTaskDetailPath("/tasks/task-123", "task-123")).toBe(true);
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
