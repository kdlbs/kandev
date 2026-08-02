import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { TaskSession } from "@/lib/types/http";
import type { SessionModelsPayload } from "@/lib/types/session-runtime-payloads";
import { registerSessionModelsHandlers } from "./session-models";

const providerModelId = "gpt-5.6-sol";
const reasoningOptionId = "reasoning_effort";
const reasoningOptionName = "Reasoning Effort";
const optionDescription = "Controls how much reasoning the model performs.";
const valueDescription = "Faster responses with less reasoning.";

function makeStore(overrides: Partial<AppState> = {}) {
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
            compactionCount: 0,
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
            compactionCount: 0,
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
      compactionCount: 0,
    });
    expect(store.getState().setSessionModels).toHaveBeenCalledWith(
      "session-1",
      expect.objectContaining({ currentModelId: "gpt-5.5" }),
    );
  });
});

describe("session.models_updated config metadata", () => {
  it("stores provider descriptions and the persisted config baseline", () => {
    const store = makeStore();
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

    handler(
      makeMessage(
        makePayload(providerModelId, {
          config_baseline: {
            model: providerModelId,
            [reasoningOptionId]: "high",
          },
          config_options: [
            {
              type: "select",
              id: reasoningOptionId,
              name: reasoningOptionName,
              description: optionDescription,
              current_value: "low",
              options: [
                {
                  value: "low",
                  name: "Low",
                  description: valueDescription,
                },
              ],
            },
          ],
        }),
      ),
    );

    expect(store.getState().sessionModels.bySessionId["session-1"]).toEqual({
      currentModelId: providerModelId,
      models: [],
      configBaseline: {
        model: providerModelId,
        [reasoningOptionId]: "high",
      },
      configOptions: [
        {
          type: "select",
          id: reasoningOptionId,
          name: reasoningOptionName,
          description: optionDescription,
          currentValue: "low",
          category: undefined,
          options: [
            {
              value: "low",
              name: "Low",
              description: valueDescription,
            },
          ],
        },
      ],
    });
  });

  it("preserves the hydrated config baseline when a provider snapshot omits it", () => {
    const configBaseline = {
      model: providerModelId,
      [reasoningOptionId]: "medium",
    };
    const store = makeStore({
      sessionModels: {
        bySessionId: {
          "session-1": {
            currentModelId: providerModelId,
            models: [],
            configOptions: [],
            configBaseline,
          },
        },
      } as AppState["sessionModels"],
    });
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

    handler(
      makeMessage(
        makePayload(providerModelId, {
          config_options: [
            {
              type: "select",
              id: reasoningOptionId,
              name: reasoningOptionName,
              current_value: "medium",
              options: [{ value: "medium", name: "Medium" }],
            },
          ],
        }),
      ),
    );

    expect(store.getState().sessionModels.bySessionId["session-1"].configBaseline).toEqual(
      configBaseline,
    );
  });
});

describe("session.models_updated persisted runtime hydration", () => {
  it("hydrates the first provider snapshot from persisted runtime metadata", () => {
    const store = makeStore({
      taskSessions: {
        items: {
          "session-1": makeTaskSession({
            runtime_config: {
              model: providerModelId,
              config_options: { [reasoningOptionId]: "medium" },
            },
            runtime_config_overrides: {
              config_options: { [reasoningOptionId]: "high" },
            },
            acp_config_baseline: {
              model: providerModelId,
              [reasoningOptionId]: "medium",
            },
          }),
        },
      },
    });
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

    handler(
      makeMessage(
        makePayload(providerModelId, {
          config_options: [
            {
              type: "select",
              id: reasoningOptionId,
              name: reasoningOptionName,
              current_value: "medium",
              options: [
                { value: "medium", name: "Medium" },
                { value: "high", name: "High" },
              ],
            },
          ],
        }),
      ),
    );

    expect(store.getState().sessionModels.bySessionId["session-1"]).toMatchObject({
      currentModelId: providerModelId,
      configBaseline: {
        model: providerModelId,
        [reasoningOptionId]: "medium",
      },
      configOptions: [expect.objectContaining({ id: reasoningOptionId, currentValue: "high" })],
    });
  });
});

describe("session.models_updated live updates", () => {
  it("trusts later provider snapshots over persisted runtime metadata", () => {
    const store = makeStore({
      taskSessions: {
        items: {
          "session-1": makeTaskSession({
            runtime_config: { config_options: { [reasoningOptionId]: "high" } },
          }),
        },
      },
      sessionModels: {
        bySessionId: {
          "session-1": {
            currentModelId: providerModelId,
            models: [],
            configOptions: [],
          },
        },
      } as AppState["sessionModels"],
    });
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

    handler(
      makeMessage(
        makePayload(providerModelId, {
          config_options: [
            {
              type: "select",
              id: reasoningOptionId,
              name: reasoningOptionName,
              current_value: "high",
              options: [{ value: "high", name: "High" }],
            },
          ],
        }),
      ),
    );
    handler(
      makeMessage(
        makePayload(providerModelId, {
          config_options: [
            {
              type: "select",
              id: reasoningOptionId,
              name: reasoningOptionName,
              current_value: "low",
              options: [{ value: "low", name: "Low" }],
            },
          ],
        }),
      ),
    );

    expect(
      store.getState().sessionModels.bySessionId["session-1"].configOptions[0]?.currentValue,
    ).toBe("low");
  });
});
