import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { fetchTaskSession } from "@/lib/api/domains/session-api";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { TaskSession } from "@/lib/types/http";
import { registerWsHandlers } from "@/lib/ws/router";
import { registerSessionWorkspaceRecoveryHandlers } from "./session-workspace-recovery";

vi.mock("@/lib/api/domains/session-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/session-api")>()),
  fetchTaskSession: vi.fn(),
}));

const TASK_ID = "task-1";
const ENVIRONMENT_ID = "environment-1";
const SESSION_IDS = ["session-1", "session-2"];

function makeStore(
  setWorkspaceRecoveryProjection = vi.fn(),
  bumpWorkspaceFilesRefresh = vi.fn(),
): StoreApi<AppState> {
  return {
    getState: () =>
      ({
        setWorkspaceRecoveryProjection,
        bumpWorkspaceFilesRefresh,
        taskSessions: { items: {} },
      }) as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

function recoveryMessage(
  state: "running" | "completed" = "running",
): BackendMessageMap["session.workspace_recovery.changed"] {
  const completed = state === "completed";
  return {
    type: "notification",
    action: "session.workspace_recovery.changed",
    payload: {
      task_id: TASK_ID,
      environment_id: ENVIRONMENT_ID,
      session_id: SESSION_IDS[0],
      session_ids: [...SESSION_IDS],
      workspace_recovery: {
        task_id: TASK_ID,
        environment_id: ENVIRONMENT_ID,
        session_id: SESSION_IDS[0],
        operation_id: "operation-1",
        attempt_id: "attempt-1",
        ownership_generation: "9",
        revision: completed ? "22" : "21",
        kind: "managed_clone_relocation",
        state,
        phase: completed ? "complete" : "restoring",
        repository_position: completed ? 2 : 1,
        repository_total: 2,
        completed_slots: completed ? 2 : 0,
        workspace_complete: completed,
        agent_ready: completed,
        runner_live: !completed,
        started_at: "2026-10-05T12:00:00Z",
        updated_at: completed ? "2026-10-05T12:00:20Z" : "2026-10-05T12:00:10Z",
      },
    },
  };
}

describe("session.workspace_recovery.changed handler", () => {
  it("is registered and forwards the environment projection to its session recipients", () => {
    const registered = registerWsHandlers(makeStore()).handlers as Record<string, unknown>;
    expect(registered["session.workspace_recovery.changed"]).toEqual(expect.any(Function));

    const setProjection = vi.fn();
    const handler = registerSessionWorkspaceRecoveryHandlers(makeStore(setProjection))[
      "session.workspace_recovery.changed"
    ]!;
    const message = recoveryMessage();

    handler(message);

    expect(setProjection).toHaveBeenCalledWith(SESSION_IDS, message.payload.workspace_recovery);
  });

  it("falls back to the initiating session when older publishers omit session_ids", () => {
    const setProjection = vi.fn();
    const handler = registerSessionWorkspaceRecoveryHandlers(makeStore(setProjection))[
      "session.workspace_recovery.changed"
    ]!;
    const message = recoveryMessage();
    delete message.payload.session_ids;

    handler(message);

    expect(setProjection).toHaveBeenCalledWith([SESSION_IDS[0]], expect.any(Object));
  });

  it("refreshes every recipient's Files tree when the shared recovery settles", () => {
    const bumpWorkspaceFilesRefresh = vi.fn();
    const handler = registerSessionWorkspaceRecoveryHandlers(
      makeStore(vi.fn(), bumpWorkspaceFilesRefresh),
    )["session.workspace_recovery.changed"]!;

    handler(recoveryMessage("completed"));

    expect(bumpWorkspaceFilesRefresh.mock.calls).toEqual(SESSION_IDS.map((id) => [id]));
  });

  it("refreshes recipient session inventory after the recovery settles", async () => {
    const currentSession = {
      id: SESSION_IDS[0],
      task_id: TASK_ID,
      task_environment_id: ENVIRONMENT_ID,
      state: "WAITING_FOR_INPUT",
      workspace_recovery: recoveryMessage("completed").payload.workspace_recovery,
      worktrees: [{ repository_id: "repo-1", worktree_path: "/old/repo" }],
    } as TaskSession;
    const latestSession = {
      ...currentSession,
      worktrees: [{ repository_id: "repo-1", worktree_path: "/new/repo" }],
    } as TaskSession;
    const setTaskSession = vi.fn();
    const setWorkspaceRecoveryProjection = vi.fn();
    const bumpWorkspaceFilesRefresh = vi.fn();
    const state = {
      taskSessions: {
        items: { [SESSION_IDS[0]]: currentSession },
        workspaceRecoveryEpochBySession: {},
      },
      setTaskSession,
      setWorkspaceRecoveryProjection,
      bumpWorkspaceFilesRefresh,
    } as unknown as AppState;
    vi.mocked(fetchTaskSession).mockResolvedValue({ session: latestSession });
    const handler = registerSessionWorkspaceRecoveryHandlers({
      getState: () => state,
      setState: vi.fn(),
      subscribe: vi.fn(),
      destroy: vi.fn(),
      getInitialState: vi.fn(),
    } as unknown as StoreApi<AppState>)["session.workspace_recovery.changed"]!;

    handler(recoveryMessage("completed"));

    await vi.waitFor(() => {
      expect(setTaskSession).toHaveBeenCalledWith(
        expect.objectContaining({ worktrees: latestSession.worktrees }),
        expect.any(Object),
      );
    });
    expect(fetchTaskSession).toHaveBeenCalledWith(SESSION_IDS[0]);
  });

  it("keeps the Files tree stable while workspace recovery is still running", () => {
    const bumpWorkspaceFilesRefresh = vi.fn();
    const handler = registerSessionWorkspaceRecoveryHandlers(
      makeStore(vi.fn(), bumpWorkspaceFilesRefresh),
    )["session.workspace_recovery.changed"]!;

    handler(recoveryMessage());

    expect(bumpWorkspaceFilesRefresh).not.toHaveBeenCalled();
  });
});
