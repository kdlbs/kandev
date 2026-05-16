import { describe, it, expect } from "vitest";
import { isFromOffice, primaryTaskRepository, type Task, type TaskRepository } from "./http";

function repo(overrides: Partial<TaskRepository>): TaskRepository {
  return {
    id: "tr-" + Math.random().toString(36).slice(2),
    task_id: "task-1",
    repository_id: "repo-x",
    base_branch: "main",
    position: 0,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  };
}

describe("primaryTaskRepository", () => {
  it("returns undefined for empty list", () => {
    expect(primaryTaskRepository(undefined)).toBeUndefined();
    expect(primaryTaskRepository([])).toBeUndefined();
  });

  it("picks lowest-position repo regardless of array order", () => {
    const result = primaryTaskRepository([
      repo({ repository_id: "back", position: 1 }),
      repo({ repository_id: "front", position: 0 }),
      repo({ repository_id: "shared", position: 2 }),
    ]);
    expect(result?.repository_id).toBe("front");
  });

  it("returns the only entry for a single-repo task", () => {
    const result = primaryTaskRepository([repo({ repository_id: "only", position: 5 })]);
    expect(result?.repository_id).toBe("only");
  });
});

function task(overrides: Partial<Task>): Task {
  return {
    id: "task-1",
    workspace_id: "ws-1",
    workflow_id: "wf-1",
    workflow_step_id: "ws-step-1",
    position: 0,
    title: "t",
    description: "",
    state: "TODO",
    priority: 0,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  };
}

describe("isFromOffice", () => {
  it("is false for null/undefined", () => {
    expect(isFromOffice(null)).toBe(false);
    expect(isFromOffice(undefined)).toBe(false);
  });

  it("is false when project_id is missing or empty", () => {
    expect(isFromOffice(task({}))).toBe(false);
    expect(isFromOffice(task({ project_id: "" }))).toBe(false);
  });

  it("is true when project_id is set (office-origin task)", () => {
    expect(isFromOffice(task({ project_id: "proj-123" }))).toBe(true);
  });
});
