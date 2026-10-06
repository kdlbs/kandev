import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import { registerWsHandlers } from "@/lib/ws/router";
import { registerSessionWorkspaceRecoveryHandlers } from "./session-workspace-recovery";

const ENVIRONMENT_ID = "environment-1";

function makeStore(setWorkspaceRecoveryProjection = vi.fn()): StoreApi<AppState> {
  return {
    getState: () => ({ setWorkspaceRecoveryProjection }) as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

describe("session.workspace_recovery.changed handler", () => {
  it("is registered and forwards the environment projection to the session store", () => {
    const registered = registerWsHandlers(makeStore()).handlers as Record<string, unknown>;
    expect(registered["session.workspace_recovery.changed"]).toEqual(expect.any(Function));

    const setProjection = vi.fn();
    const handler = registerSessionWorkspaceRecoveryHandlers(makeStore(setProjection))[
      "session.workspace_recovery.changed"
    ]!;
    const message: BackendMessageMap["session.workspace_recovery.changed"] = {
      type: "notification",
      action: "session.workspace_recovery.changed",
      payload: {
        task_id: "task-1",
        environment_id: ENVIRONMENT_ID,
        session_id: "session-1",
        session_ids: ["session-1", "session-2"],
        workspace_recovery: {
          task_id: "task-1",
          environment_id: ENVIRONMENT_ID,
          session_id: "session-1",
          operation_id: "operation-1",
          attempt_id: "attempt-1",
          ownership_generation: "9",
          revision: "21",
          kind: "managed_clone_relocation",
          state: "running",
          phase: "restoring",
          repository_position: 1,
          repository_total: 2,
          completed_slots: 0,
          workspace_complete: false,
          agent_ready: false,
          runner_live: true,
          started_at: "2026-10-05T12:00:00Z",
          updated_at: "2026-10-05T12:00:10Z",
        },
      },
    };

    handler(message);

    expect(setProjection).toHaveBeenCalledWith(
      ["session-1", "session-2"],
      message.payload.workspace_recovery,
    );
  });

  it("falls back to the initiating session when older publishers omit session_ids", () => {
    const setProjection = vi.fn();
    const handler = registerSessionWorkspaceRecoveryHandlers(makeStore(setProjection))[
      "session.workspace_recovery.changed"
    ]!;
    handler({
      type: "notification",
      action: "session.workspace_recovery.changed",
      payload: {
        task_id: "task-1",
        environment_id: ENVIRONMENT_ID,
        session_id: "session-1",
        workspace_recovery: {
          task_id: "task-1",
          environment_id: ENVIRONMENT_ID,
          session_id: "session-1",
          operation_id: "operation-1",
          attempt_id: "attempt-1",
          ownership_generation: "9",
          revision: "21",
          kind: "managed_clone_relocation",
          state: "running",
          phase: "checking",
          repository_position: 0,
          repository_total: 1,
          completed_slots: 0,
          workspace_complete: false,
          agent_ready: false,
          runner_live: true,
          started_at: "2026-10-05T12:00:00Z",
          updated_at: "2026-10-05T12:00:00Z",
        },
      },
    });
    expect(setProjection).toHaveBeenCalledWith(["session-1"], expect.any(Object));
  });
});
