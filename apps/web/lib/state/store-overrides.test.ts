import { describe, expect, it } from "vitest";
import { createAppStore, type AppState } from "./store";

function commit(): AppState["sessionCommits"]["byEnvironmentId"][string][number] {
  return {
    id: "commit-1",
    session_id: "session-1",
    commit_sha: "abc123",
    parent_sha: "base123",
    author_name: "Agent",
    author_email: "agent@example.com",
    commit_message: "test",
    committed_at: "2026-01-01T00:00:00Z",
    files_changed: 1,
    insertions: 2,
    deletions: 1,
    created_at: "2026-01-01T00:00:01Z",
  };
}

describe("createAppStore initial state overrides", () => {
  it("reasserts bootstrapped session runtime and plan state after slice defaults", () => {
    const store = createAppStore({
      environmentIdBySessionId: { "session-1": "env-1" },
      sessionCommits: {
        byEnvironmentId: { "env-1": [commit()] },
        loading: { "env-1": true },
        refetchTrigger: { "env-1": 2 },
      },
      taskPlans: {
        previewRevisionIdByTaskId: { "task-1": "rev-1" },
        comparePairByTaskId: { "task-1": ["rev-1", null] },
        lastSeenUpdatedAtByTaskId: { "task-1": "2026-01-01T00:00:00Z" },
      },
    });

    const state = store.getState();
    expect(state.environmentIdBySessionId).toEqual({ "session-1": "env-1" });
    expect(state.sessionCommits.byEnvironmentId["env-1"]).toHaveLength(1);
    expect(state.sessionCommits.loading["env-1"]).toBe(true);
    expect(state.sessionCommits.refetchTrigger["env-1"]).toBe(2);
    expect(state.taskPlans.previewRevisionIdByTaskId["task-1"]).toBe("rev-1");
    expect(state.taskPlans.lastSeenUpdatedAtByTaskId["task-1"]).toBe("2026-01-01T00:00:00Z");
  });
});
