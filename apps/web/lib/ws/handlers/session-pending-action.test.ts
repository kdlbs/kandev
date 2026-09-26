import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import { registerWsHandlers } from "@/lib/ws/router";
import { registerSessionPendingActionHandlers } from "./session-pending-action";

function makeStore(): StoreApi<AppState> {
  return {
    getState: () => ({}) as AppState,
    setState: () => undefined,
    subscribe: () => () => undefined,
    destroy: () => undefined,
    getInitialState: () => ({}) as AppState,
  } as StoreApi<AppState>;
}

function makeHandlerStore(setTaskSessionPendingAction = vi.fn()): StoreApi<AppState> {
  return {
    getState: () => ({ setTaskSessionPendingAction }) as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

describe("session.pending_action_changed handler", () => {
  it("is registered in the websocket handler registry", () => {
    const handlers = registerWsHandlers(makeStore()).handlers as Record<string, unknown>;

    expect(handlers["session.pending_action_changed"]).toEqual(expect.any(Function));
  });

  it("applies the complete compact projection through the revision-aware store action", () => {
    const setTaskSessionPendingAction = vi.fn();
    const store = makeHandlerStore(setTaskSessionPendingAction);
    const handler = registerSessionPendingActionHandlers(store)["session.pending_action_changed"]!;
    const message: BackendMessageMap["session.pending_action_changed"] = {
      id: "event-1",
      type: "notification",
      action: "session.pending_action_changed",
      payload: {
        workspace_id: "workspace-1",
        task_id: "task-1",
        session_id: "session-1",
        pending_action: null,
        pending_action_revision: { epoch: "7", sequence: 12 },
      },
    };

    handler(message);

    expect(setTaskSessionPendingAction).toHaveBeenCalledWith(
      "session-1",
      null,
      {
        epoch: "7",
        sequence: 12,
      },
      "task-1",
    );
  });

  it("ignores an event without complete workspace identity", () => {
    const setTaskSessionPendingAction = vi.fn();
    const store = makeHandlerStore(setTaskSessionPendingAction);
    const handler = registerSessionPendingActionHandlers(store)["session.pending_action_changed"]!;

    handler({
      id: "event-1",
      type: "notification",
      action: "session.pending_action_changed",
      payload: {
        workspace_id: "",
        task_id: "task-1",
        session_id: "session-1",
        pending_action: "permission",
        pending_action_revision: { epoch: "7", sequence: 12 },
      },
    });

    expect(setTaskSessionPendingAction).not.toHaveBeenCalled();
  });
});
