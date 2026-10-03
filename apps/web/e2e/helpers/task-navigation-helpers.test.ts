import { describe, expect, it, vi } from "vitest";
import type { ApiClient } from "./api-client";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import { seedNavigationTasks } from "../tests/task/task-navigation-helpers";

vi.mock("@playwright/test", async () => ({
  expect: (await import("vitest")).expect,
}));

vi.mock("../fixtures/test-base", () => ({
  resetSeedRepositoryCheckout: vi.fn(),
}));

vi.mock("./git-helper", () => ({
  GitHelper: class {
    exec() {
      return "";
    }
    createFile() {}
    stageAll() {}
  },
  makeGitEnv: () => ({}),
  createStandardProfile: async () => ({ id: "navigation-profile" }),
}));

describe("seedNavigationTasks", () => {
  it.each([
    { requested: undefined, expected: "worktree-profile" },
    { requested: "explicit-profile", expected: "explicit-profile" },
  ])("settles preparation and selects $expected", async ({ requested, expected }) => {
    let preparingSession: string | undefined;
    const sessions = new Map<string, string>();
    const api = {
      createTaskWithAgent: vi.fn(async (..._args: unknown[]) => {
        if (preparingSession) throw new Error("Shared checkout index.lock is held");
        const id = `task-${sessions.size + 1}`;
        const sessionId = `session-${id}`;
        sessions.set(id, sessionId);
        preparingSession = sessionId;
        return { id, session_id: sessionId };
      }),
      listTaskSessions: vi.fn(async (taskId: string) => {
        const id = sessions.get(taskId)!;
        preparingSession = undefined;
        return { sessions: [{ id, state: "WAITING_FOR_INPUT" }] };
      }),
    };

    const tasks = await seedNavigationTasks(
      api as unknown as ApiClient,
      {
        workspaceId: "workspace",
        workflowId: "workflow",
        startStepId: "start",
        repositoryId: "repository",
        worktreeExecutorProfileId: "worktree-profile",
      } as SeedData,
      { tmpDir: "/navigation-fixture" } as BackendContext,
      requested,
    );

    expect(tasks.map((task) => task.id)).toEqual(["task-1", "task-2"]);
    expect(api.listTaskSessions).toHaveBeenCalledTimes(2);
    for (const call of api.createTaskWithAgent.mock.calls) {
      expect(call[3]).toEqual(expect.objectContaining({ executor_profile_id: expected }));
    }
    expect(preparingSession).toBeUndefined();
  });
});
