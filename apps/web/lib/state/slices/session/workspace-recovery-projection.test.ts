import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import {
  sessionId,
  taskId,
  type TaskSession,
  type WorkspaceRecoveryProjection,
} from "@/lib/types/http";
import { mergeTaskSession, mergeWorkspaceRecoveryProjection } from "./session-merge";
import { captureTaskSessionHydrationEpoch } from "./hydration-epochs";

const TASK_ID = taskId("task-recovery");
const ENVIRONMENT_ID = "environment-recovery";
const SESSION_ID = sessionId("session-recovery");

function projection(
  overrides: Partial<WorkspaceRecoveryProjection> = {},
): WorkspaceRecoveryProjection {
  return {
    task_id: TASK_ID,
    environment_id: ENVIRONMENT_ID,
    session_id: SESSION_ID,
    operation_id: "operation-1",
    attempt_id: "attempt-1",
    ownership_generation: "7",
    revision: "12",
    kind: "managed_clone_relocation",
    state: "running",
    phase: "snapshotting",
    repository_id: "repository-1",
    repository_position: 1,
    repository_total: 2,
    completed_slots: 0,
    workspace_complete: false,
    agent_ready: false,
    runner_live: true,
    started_at: "2026-10-05T12:00:00Z",
    updated_at: "2026-10-05T12:00:10Z",
    ...overrides,
  };
}

function session(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    task_environment_id: ENVIRONMENT_ID,
    state: "FAILED",
    started_at: "2026-10-05T11:00:00Z",
    updated_at: "2026-10-05T11:00:00Z",
    ...overrides,
  };
}

describe("workspace recovery projection ordering", () => {
  it("rejects older attempts and revisions while accepting a newer owner generation", () => {
    const current = projection();
    expect(
      mergeWorkspaceRecoveryProjection(
        current,
        projection({ revision: "11", phase: "checking" }),
        ENVIRONMENT_ID,
      ),
    ).toBe(current);
    expect(
      mergeWorkspaceRecoveryProjection(
        current,
        projection({ ownership_generation: "6", revision: "99" }),
        ENVIRONMENT_ID,
      ),
    ).toBe(current);
    expect(
      mergeWorkspaceRecoveryProjection(
        current,
        projection({ ownership_generation: "8", revision: "1" }),
        ENVIRONMENT_ID,
      )?.ownership_generation,
    ).toBe("8");
  });

  it("preserves an existing projection when an older response omits it", () => {
    const current = session({ workspace_recovery: projection() });
    const merged = mergeTaskSession(current, session({ workspace_recovery: undefined }));
    expect(merged.workspace_recovery).toBe(current.workspace_recovery);
  });

  it("caches notifications that arrive before session hydration", () => {
    const store = createAppStore();
    const current = projection();
    store.getState().setWorkspaceRecoveryProjection([SESSION_ID], current);
    store.getState().setTaskSessionsForTask(TASK_ID, [session({ workspace_recovery: null })], {});

    expect(store.getState().taskSessions.items[SESSION_ID]?.workspace_recovery).toEqual(current);
    expect(
      store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID]?.[0].workspace_recovery,
    ).toEqual(current);
  });

  it("does not let a response started before a notification clear the newer projection", () => {
    const store = createAppStore();
    store.getState().setTaskSession(session({ workspace_recovery: null }));
    const requestEpoch = captureTaskSessionHydrationEpoch(store.getState(), SESSION_ID);
    const current = projection({ revision: "13", phase: "verifying_snapshot" });
    store.getState().setWorkspaceRecoveryProjection([SESSION_ID], current);

    store.getState().setTaskSession(session({ workspace_recovery: null }), requestEpoch);

    expect(store.getState().taskSessions.items[SESSION_ID]?.workspace_recovery).toEqual(current);
  });

  it("applies a terminal liveness update to every loaded session in the environment", () => {
    const store = createAppStore();
    const siblingID = sessionId("session-sibling");
    store.getState().setTaskSession(session());
    store.getState().setTaskSession(session({ id: siblingID }));
    store.getState().setWorkspaceRecoveryProjection([SESSION_ID, siblingID], projection());

    const settled = projection({
      state: "failed",
      phase: "resuming",
      revision: "13",
      runner_live: false,
    });
    store.getState().setWorkspaceRecoveryProjection([SESSION_ID, siblingID], settled);

    expect(store.getState().taskSessions.items[SESSION_ID]?.workspace_recovery).toEqual(settled);
    expect(store.getState().taskSessions.items[siblingID]?.workspace_recovery).toEqual(settled);
  });

  it("does not attach an environment projection to a session bound elsewhere", () => {
    const store = createAppStore();
    const otherEnvironmentSession = session({ task_environment_id: "environment-other" });
    store.getState().setTaskSession(otherEnvironmentSession);
    store.getState().setWorkspaceRecoveryProjection([SESSION_ID], projection());

    expect(store.getState().taskSessions.items[SESSION_ID]?.workspace_recovery).toBeUndefined();
  });
});
