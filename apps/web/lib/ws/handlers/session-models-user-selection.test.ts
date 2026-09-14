import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { TaskSession } from "@/lib/types/http";
import type { SessionModelsPayload } from "@/lib/types/session-runtime-payloads";
import { registerSessionModelsHandlers } from "./session-models";

const providerModelId = "gpt-5.6-sol";
const providerModelName = "GPT-5.6 Sol";

function makeStore(overrides: Partial<AppState> = {}): StoreApi<AppState> {
  const state = {
    activeModel: { bySessionId: {} },
    contextWindow: { bySessionId: {} },
    sessionModels: { bySessionId: {} },
    taskSessions: { items: {} },
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

function makeTaskSession(metadata: Record<string, unknown>): TaskSession {
  return {
    id: "session-1",
    task_id: "task-1",
    state: "WAITING_FOR_INPUT",
    metadata,
    started_at: "2026-06-11T00:00:00.000Z",
    updated_at: "2026-06-11T00:00:00.000Z",
  } as TaskSession;
}

describe("session.models_updated user selection convergence", () => {
  const pickedModelId = "gpt-5.3-codex-spark";
  const laterModelId = "gpt-5.4-mini";

  // After a page reload the store has no hydration bookkeeping, so persisted
  // runtime metadata is authoritative until a payload confirms it. A model the
  // user just picked in this store is such a confirmation: without it, the
  // convergence event for the user's own selection is overwritten by the
  // stale cached override and the picker snaps back to the previous model.
  it("lets a convergence event for the user's own selection win over stale persisted metadata", () => {
    const store = makeStore({
      activeModel: { bySessionId: { "session-1": pickedModelId } } as AppState["activeModel"],
      taskSessions: {
        items: {
          "session-1": makeTaskSession({
            runtime_config: { model: providerModelId },
            runtime_config_overrides: { model: providerModelId },
          }),
        },
      },
      sessionModels: {
        bySessionId: {
          "session-1": {
            currentModelId: providerModelId,
            models: [],
            configOptions: [],
            configOptionsSettled: true,
          },
        },
      } as AppState["sessionModels"],
    });
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

    handler(
      makeMessage(
        makePayload(pickedModelId, {
          config_options: [
            {
              type: "select",
              id: "model",
              name: "Model",
              category: "model",
              current_value: pickedModelId,
              options: [
                { value: providerModelId, name: providerModelName },
                { value: pickedModelId, name: "Codex Spark" },
              ],
            },
          ],
        }),
      ),
    );

    const entry = store.getState().sessionModels.bySessionId["session-1"];
    expect(entry.currentModelId).toBe(pickedModelId);
    expect(entry.configOptions[0]?.currentValue).toBe(pickedModelId);
  });

  it("keeps trusting the payload for the switch after the user's selection converged", () => {
    const store = makeStore({
      activeModel: { bySessionId: { "session-1": pickedModelId } } as AppState["activeModel"],
      taskSessions: {
        items: {
          "session-1": makeTaskSession({
            runtime_config: { model: providerModelId },
            runtime_config_overrides: { model: providerModelId },
          }),
        },
      },
      sessionModels: {
        bySessionId: {
          "session-1": {
            currentModelId: providerModelId,
            models: [],
            configOptions: [],
            configOptionsSettled: true,
          },
        },
      } as AppState["sessionModels"],
    });
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

    handler(makeMessage(makePayload(pickedModelId)));
    // The provider re-reports the same session a moment later; the stale
    // override must not resurrect the pre-switch model.
    handler(makeMessage(makePayload(laterModelId)));

    expect(store.getState().sessionModels.bySessionId["session-1"].currentModelId).toBe(
      laterModelId,
    );
  });
});

describe("session.models_updated startup guard", () => {
  const pickedModelId = "gpt-5.3-codex-spark";

  it("keeps persisted state during unsettled startup when active model matches", () => {
    const store = makeStore({
      activeModel: { bySessionId: { "session-1": pickedModelId } } as AppState["activeModel"],
      taskSessions: {
        items: {
          "session-1": {
            ...makeTaskSession({ runtime_config: { model: providerModelId } }),
            state: "STARTING",
          },
        },
      },
      sessionModels: {
        bySessionId: {
          "session-1": {
            currentModelId: providerModelId,
            models: [],
            configOptions: [],
            configOptionsSettled: true,
          },
        },
      } as AppState["sessionModels"],
    });
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

    handler(
      makeMessage(
        makePayload(pickedModelId, {
          config_options_settled: false,
          config_options: [
            {
              type: "select",
              id: "model",
              name: "Model",
              category: "model",
              current_value: pickedModelId,
              options: [{ value: pickedModelId, name: "Codex Spark" }],
            },
          ],
        }),
      ),
    );

    expect(store.getState().sessionModels.bySessionId["session-1"]).toMatchObject({
      currentModelId: providerModelId,
      configOptions: [expect.objectContaining({ id: "model", currentValue: providerModelId })],
    });
  });
});
