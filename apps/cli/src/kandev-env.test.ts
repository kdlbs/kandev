import os from "node:os";
import path from "node:path";

import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { KANDEV_TASKS_DIR } from "./constants";
import { isInsideKandevTask } from "./kandev-env";

describe("isInsideKandevTask", () => {
  const originalTaskId = process.env.KANDEV_TASK_ID;

  beforeEach(() => {
    delete process.env.KANDEV_TASK_ID;
  });

  afterEach(() => {
    if (originalTaskId === undefined) {
      delete process.env.KANDEV_TASK_ID;
    } else {
      process.env.KANDEV_TASK_ID = originalTaskId;
    }
  });

  it("returns true when KANDEV_TASK_ID is set regardless of repo path", () => {
    process.env.KANDEV_TASK_ID = "some-id";
    expect(isInsideKandevTask("/any/path")).toBe(true);
  });

  it("returns true for a repoRoot under the kandev tasks dir", () => {
    const repoRoot = path.join(KANDEV_TASKS_DIR, "task-123", "kandev");
    expect(isInsideKandevTask(repoRoot)).toBe(true);
  });

  it("returns false for a repoRoot outside the kandev tasks dir", () => {
    expect(isInsideKandevTask("/home/user/projects/kandev")).toBe(false);
  });

  it("does not match a sibling directory that shares the 'tasks' prefix", () => {
    // Boundary: ~/.kandev/tasks-extra/... must NOT be treated as a task workspace.
    // The `path.sep` suffix on KANDEV_TASKS_DIR is the only guard against this.
    const adjacent = path.join(os.homedir(), ".kandev", "tasks-extra", "proj");
    expect(isInsideKandevTask(adjacent)).toBe(false);
  });

  it("does not match the tasks dir itself without a subdirectory", () => {
    expect(isInsideKandevTask(KANDEV_TASKS_DIR)).toBe(false);
  });
});
