import { afterEach, describe, expect, it, vi } from "vitest";
import { linkToTask, linkToTasks, replaceTaskUrl } from "./links";

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
