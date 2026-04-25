import { describe, it, expect } from "vitest";
import { primaryTaskRepository, type TaskRepository } from "./http";

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
