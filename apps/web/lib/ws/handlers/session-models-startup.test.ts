import { expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { TaskSession } from "@/lib/types/http";
import type { SessionModelsPayload } from "@/lib/types/session-runtime-payloads";
import { registerSessionModelsHandlers } from "./session-models";

const providerModelId = "gpt-5.6-sol";
const providerModelName = "GPT-5.6 Sol";
const lunaModelId = "gpt-5.6-luna";
const reasoningOptionId = "reasoning_effort";
const reasoningOptionName = "Reasoning Effort";

function makeStore(overrides: Partial<AppState> = {}): StoreApi<AppState> {
  const state = {
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

function makeStartupPayload(
  currentModelId: string,
  configModelId: string,
  settled: boolean,
  effort: string,
): SessionModelsPayload {
  return makePayload(currentModelId, {
    config_options_settled: settled,
    config_options: [
      {
        type: "select",
        id: "model",
        name: "Model",
        current_value: configModelId,
        options: [
          { value: providerModelId, name: providerModelName },
          { value: lunaModelId, name: "GPT-5.6 Luna" },
        ],
      },
      {
        type: "select",
        id: reasoningOptionId,
        name: reasoningOptionName,
        current_value: effort,
        options: [
          { value: "high", name: "High" },
          { value: "low", name: "Low" },
        ],
      },
    ],
  });
}

function expectCurrentModel(store: StoreApi<AppState>, modelId: string): void {
  expect(store.getState().sessionModels.bySessionId["session-1"]).toMatchObject({
    currentModelId: modelId,
    configOptions: expect.arrayContaining([
      expect.objectContaining({ id: "model", currentValue: modelId }),
    ]),
  });
}

function expectPersistedModel(store: StoreApi<AppState>): void {
  expectCurrentModel(store, providerModelId);
}

it("keeps persisted model state until a settled startup payload arrives", () => {
  const store = makeStore({
    taskSessions: {
      items: {
        "session-1": {
          ...makeTaskSession({ runtime_config: { model: providerModelId } }),
          state: "STARTING",
        },
      },
    },
  });
  const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

  handler(makeMessage(makeStartupPayload(providerModelId, lunaModelId, false, "low")));
  expectPersistedModel(store);

  handler(makeMessage(makeStartupPayload(lunaModelId, lunaModelId, false, "low")));
  expectPersistedModel(store);

  handler(makeMessage(makeStartupPayload(providerModelId, providerModelId, true, "high")));
  expectPersistedModel(store);
  expect(store.getState().sessionModels.bySessionId["session-1"]).toMatchObject({
    configOptionsSettled: true,
  });

  store.getState().taskSessions.items["session-1"].state = "WAITING_FOR_INPUT";
  handler(makeMessage(makeStartupPayload(lunaModelId, lunaModelId, false, "low")));
  const liveState = store.getState().sessionModels.bySessionId["session-1"];
  expect(liveState.currentModelId).toBe(lunaModelId);
  expect(liveState.configOptions.find((option) => option.id === "model")?.currentValue).toBe(
    lunaModelId,
  );
});

it("restores persisted runtime after a hydrated session restarts", () => {
  const store = makeStore({
    contextWindow: {
      bySessionId: {
        "session-1": {
          size: 100,
          used: 40,
          remaining: 60,
          efficiency: 60,
          compactionCount: 0,
        },
      },
    },
    taskSessions: {
      items: {
        "session-1": makeTaskSession({ runtime_config: { model: providerModelId } }),
      },
    },
  });
  const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

  handler(makeMessage(makeStartupPayload(providerModelId, providerModelId, true, "high")));
  expectPersistedModel(store);

  store.getState().taskSessions.items["session-1"].state = "STARTING";
  handler(makeMessage(makeStartupPayload(lunaModelId, lunaModelId, false, "low")));
  expectPersistedModel(store);
  expect(store.getState().clearContextWindow).not.toHaveBeenCalled();
  expect(store.getState().contextWindow.bySessionId["session-1"]).toBeDefined();

  handler(makeMessage(makeStartupPayload(providerModelId, providerModelId, true, "high")));
  expectPersistedModel(store);
});

it("hydrates from a settled startup payload when the model differs from persisted runtime", () => {
  const store = makeStore({
    taskSessions: {
      items: {
        "session-1": {
          ...makeTaskSession({ runtime_config: { model: providerModelId } }),
          state: "STARTING",
        },
      },
    },
  });
  const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

  handler(makeMessage(makeStartupPayload(lunaModelId, lunaModelId, true, "low")));
  expectCurrentModel(store, lunaModelId);
  expect(store.getState().sessionModels.bySessionId["session-1"]).toMatchObject({
    configOptionsSettled: true,
  });

  store.getState().taskSessions.items["session-1"].state = "WAITING_FOR_INPUT";
  handler(makeMessage(makeStartupPayload(providerModelId, providerModelId, false, "high")));
  expectCurrentModel(store, providerModelId);
});

it("preserves populated flat models when a partial replay drops them", () => {
  const existingModels = [
    { modelId: providerModelId, name: providerModelName },
    { modelId: lunaModelId, name: "GPT-5.6 Luna" },
  ];
  const store = makeStore({
    sessionModels: {
      bySessionId: {
        "session-1": {
          currentModelId: providerModelId,
          models: existingModels,
          configOptions: [],
        },
      },
    } as AppState["sessionModels"],
  });
  const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

  handler(makeMessage(makePayload(providerModelId, { models: [], config_options: [] })));

  expect(store.getState().sessionModels.bySessionId["session-1"].models).toEqual(existingModels);
});

it("clears populated flat models when a settled update removes them", () => {
  const store = makeStore({
    sessionModels: {
      bySessionId: {
        "session-1": {
          currentModelId: providerModelId,
          models: [{ modelId: providerModelId, name: providerModelName }],
          configOptions: [],
        },
      },
    } as AppState["sessionModels"],
  });
  const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

  handler(
    makeMessage(
      makePayload(providerModelId, {
        models: [],
        config_options: [],
        config_options_settled: true,
      }),
    ),
  );

  expect(store.getState().sessionModels.bySessionId["session-1"].models).toEqual([]);
});
