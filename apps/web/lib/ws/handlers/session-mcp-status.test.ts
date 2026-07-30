import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import { registerSessionMCPStatusHandlers } from "./session-mcp-status";

function makeStore() {
  const state = {
    setSessionMCPStatus: vi.fn(),
  } as unknown as AppState;

  return {
    getState: () => state,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

function makeMessage(
  payload: BackendMessageMap["session.mcp_status_updated"]["payload"],
): BackendMessageMap["session.mcp_status_updated"] {
  return {
    id: "message-1",
    type: "notification",
    action: "session.mcp_status_updated",
    payload,
  };
}

describe("session.mcp_status_updated handler", () => {
  it("stores an attachment report for its exact session", () => {
    const store = makeStore();
    const handler = registerSessionMCPStatusHandlers(store)["session.mcp_status_updated"]!;
    const history = {
      version: 1,
      current: {
        attachment_attempt_id: "attempt-1",
        started_at: "2026-07-30T00:00:00Z",
        servers: [{ name: "kandev", status: "active" as const }],
      },
    };

    handler(
      makeMessage({
        task_id: "task-1",
        session_id: "session-1",
        history,
        timestamp: "2026-07-30T00:00:00Z",
      }),
    );

    expect(store.getState().setSessionMCPStatus).toHaveBeenCalledWith("session-1", history);
  });

  it("ignores malformed reports without an attempt identity", () => {
    const store = makeStore();
    const handler = registerSessionMCPStatusHandlers(store)["session.mcp_status_updated"]!;

    handler(
      makeMessage({
        task_id: "task-1",
        session_id: "session-1",
        history: { version: 1, current: { attachment_attempt_id: "", started_at: "" } },
        timestamp: "2026-07-30T00:00:00Z",
      }),
    );

    expect(store.getState().setSessionMCPStatus).not.toHaveBeenCalled();
  });
});
