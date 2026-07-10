import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { SessionModelsPayload } from "@/lib/types/session-runtime-payloads";
import { registerSessionModelsHandlers } from "./session-models";

function makeStore(overrides: Partial<AppState> = {}) {
  const state = {
    activeModel: { bySessionId: {} },
    contextWindow: { bySessionId: {} },
    sessionModels: { bySessionId: {} },
    setActiveModel: vi.fn(),
    ...overrides,
  } as unknown as AppState;
  state.setSessionModels = vi.fn((sessionId, data) => {
    state.sessionModels.bySessionId[sessionId] = data;
  });
  state.clearContextWindow = vi.fn((sessionId) => {
    delete state.contextWindow.bySessionId[sessionId];
  });

  return {
    getState: () => state,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

function makePayload(
  currentModelId: string,
  overrides: Partial<SessionModelsPayload> = {},
): SessionModelsPayload {
  return {
    task_id: "task-1",
    session_id: "session-1",
    agent_id: "agent-1",
    current_model_id: currentModelId,
    models: [],
    config_options: [],
    timestamp: "2026-06-11T00:00:00.000Z",
    ...overrides,
  };
}

function makeMessage(payload: SessionModelsPayload): BackendMessageMap["session.models_updated"] {
  return {
    id: "message-1",
    type: "notification",
    action: "session.models_updated",
    payload,
  };
}

describe("session.models_updated handler", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("clears stale context window when the current model changes", () => {
    const store = makeStore({
      contextWindow: {
        bySessionId: {
          "session-1": {
            size: 258400,
            used: 114000,
            remaining: 144400,
            efficiency: 44,
          },
        },
      } as AppState["contextWindow"],
      sessionModels: {
        bySessionId: {
          "session-1": {
            currentModelId: "gpt-5.5",
            models: [],
            configOptions: [],
          },
        },
      } as AppState["sessionModels"],
    });
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

    handler(makeMessage(makePayload("gpt-5.3-codex-spark")));

    expect(store.getState().clearContextWindow).toHaveBeenCalledWith("session-1");
    expect(store.getState().contextWindow.bySessionId["session-1"]).toBeUndefined();
    expect(store.getState().setSessionModels).toHaveBeenCalledWith(
      "session-1",
      expect.objectContaining({ currentModelId: "gpt-5.3-codex-spark" }),
    );
  });

  it("keeps context window when the current model is unchanged", () => {
    const store = makeStore({
      contextWindow: {
        bySessionId: {
          "session-1": {
            size: 258400,
            used: 114000,
            remaining: 144400,
            efficiency: 44,
          },
        },
      } as AppState["contextWindow"],
      sessionModels: {
        bySessionId: {
          "session-1": {
            currentModelId: "gpt-5.5",
            models: [],
            configOptions: [],
          },
        },
      } as AppState["sessionModels"],
    });
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

    handler(makeMessage(makePayload("gpt-5.5")));

    expect(store.getState().clearContextWindow).not.toHaveBeenCalled();
    expect(store.getState().contextWindow.bySessionId["session-1"]).toEqual({
      size: 258400,
      used: 114000,
      remaining: 144400,
      efficiency: 44,
    });
    expect(store.getState().setSessionModels).toHaveBeenCalledWith(
      "session-1",
      expect.objectContaining({ currentModelId: "gpt-5.5" }),
    );
  });
});
