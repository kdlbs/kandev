import { beforeEach, describe, expect, it } from "vitest";
import {
  getRecentTasks,
  MAX_RECENT_TASKS,
  RECENT_TASKS_STORAGE_KEY,
  removeRecentTask,
  setRecentTasks,
  upsertRecentTask,
  type RecentTaskEntry,
} from "./recent-tasks";

function entry(taskId: string, title = taskId): RecentTaskEntry {
  return {
    taskId,
    title,
    visitedAt: `2026-05-02T10:00:${taskId.padStart(2, "0")}Z`,
    taskState: "TODO",
    sessionState: null,
    repositoryPath: null,
    workflowId: "workflow-1",
    workflowName: "Main Workflow",
    workflowStepTitle: "Todo",
    workspaceId: "workspace-1",
  };
}

describe("recent task storage", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("returns an empty list for malformed storage", () => {
    window.localStorage.setItem(RECENT_TASKS_STORAGE_KEY, "{not json");

    expect(getRecentTasks()).toEqual([]);
  });

  it("stores newest visits first and deduplicates by task id", () => {
    upsertRecentTask(entry("1", "First"));
    upsertRecentTask(entry("2", "Second"));
    const result = upsertRecentTask(entry("1", "First updated"));

    expect(result.map((item) => item.taskId)).toEqual(["1", "2"]);
    expect(result[0]?.title).toBe("First updated");
    expect(getRecentTasks().map((item) => item.taskId)).toEqual(["1", "2"]);
  });

  it("caps the list to the maximum recent task count", () => {
    for (let index = 0; index < MAX_RECENT_TASKS + 2; index += 1) {
      upsertRecentTask(entry(String(index), `Task ${index}`));
    }

    const result = getRecentTasks();
    expect(result).toHaveLength(MAX_RECENT_TASKS);
    expect(result[0]?.taskId).toBe(String(MAX_RECENT_TASKS + 1));
    expect(result.some((item) => item.taskId === "0")).toBe(false);
  });

  it("removes a task from stored recents", () => {
    setRecentTasks([entry("1"), entry("2")]);

    const result = removeRecentTask("1");

    expect(result.map((item) => item.taskId)).toEqual(["2"]);
    expect(getRecentTasks().map((item) => item.taskId)).toEqual(["2"]);
  });
});
