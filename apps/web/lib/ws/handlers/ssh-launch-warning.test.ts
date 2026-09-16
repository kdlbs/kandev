import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap, LaunchWarningPayload } from "@/lib/types/backend";
import { registerSSHLaunchWarningHandlers } from "./ssh-launch-warning";

const SESSION_ID = "session-1";
const TASK_ID = "task-1";

function makeStore() {
  const state = {
    launchWarning: { bySessionId: {} as Record<string, unknown> },
    setLaunchWarning: vi.fn(),
  } as unknown as AppState;

  return {
    getState: () => state,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

function makePayload(overrides: Partial<LaunchWarningPayload> = {}): LaunchWarningPayload {
  return {
    task_id: TASK_ID,
    session_id: SESSION_ID,
    executor_id: "executor-1",
    host: "10.0.0.5",
    state: "unreachable",
    reason: "timeout",
    last_success_at: "2026-06-10T00:00:00.000Z",
    timestamp: "2026-06-11T00:00:00.000Z",
    ...overrides,
  };
}

function makeMessage(payload: LaunchWarningPayload): BackendMessageMap["session.launch.warning"] {
  return {
    id: "message-1",
    type: "notification",
    action: "session.launch.warning",
    payload,
  };
}

describe("session.launch.warning handler", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("stores a normalized entry keyed by session id", () => {
    const store = makeStore();
    const handler = registerSSHLaunchWarningHandlers(store)["session.launch.warning"]!;

    handler(makeMessage(makePayload()));

    expect(store.getState().setLaunchWarning).toHaveBeenCalledWith(SESSION_ID, {
      executorId: "executor-1",
      host: "10.0.0.5",
      state: "unreachable",
      reason: "timeout",
      lastSuccessAt: "2026-06-10T00:00:00.000Z",
    });
  });

  it("omits lastSuccessAt when the record has never succeeded", () => {
    const store = makeStore();
    const handler = registerSSHLaunchWarningHandlers(store)["session.launch.warning"]!;

    handler(makeMessage(makePayload({ last_success_at: undefined })));

    expect(store.getState().setLaunchWarning).toHaveBeenCalledWith(
      SESSION_ID,
      expect.objectContaining({ lastSuccessAt: undefined }),
    );
  });

  it("ignores payloads without a session id", () => {
    const store = makeStore();
    const handler = registerSSHLaunchWarningHandlers(store)["session.launch.warning"]!;

    handler(makeMessage(makePayload({ session_id: "" })));

    expect(store.getState().setLaunchWarning).not.toHaveBeenCalled();
  });
});
