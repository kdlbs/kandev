import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { TaskSession } from "@/lib/types/http";
import type { SessionModelsPayload } from "@/lib/types/session-runtime-payloads";
import { mapAvailableCommandToSlashCommand } from "@/components/task/chat/slash-command-types";
import { registerSessionModelsHandlers } from "./session-models";

const providerModelId = "gpt-5.6-sol";
const executionId = "execution-1";
const collaborationModeId = "collaboration_mode";
const collaborationModeName = "Collaboration Mode";

function modeOption(value: string) {
  return {
    type: "select",
    id: collaborationModeId,
    name: collaborationModeName,
    current_value: value,
    options: [],
  };
}

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

describe("confirmed session configuration", () => {
  it("keeps provider values separate from preference overlays", () => {
    const store = makeStore({
      taskSessions: {
        items: {
          "session-1": makeTaskSession({
            runtime_config_overrides: { config_options: { [collaborationModeId]: "default" } },
          }),
        },
      },
    });
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

    handler(
      makeMessage(
        makePayload(providerModelId, {
          config_options_settled: true,
          config_options: [
            {
              type: "select",
              id: collaborationModeId,
              name: collaborationModeName,
              current_value: "plan",
              options: [],
            },
          ],
        }),
      ),
    );

    expect(store.getState().sessionModels.bySessionId["session-1"]).toMatchObject({
      confirmedConfigOptions: { [collaborationModeId]: "plan" },
      configOptions: [expect.objectContaining({ id: collaborationModeId, currentValue: "plan" })],
    });
  });
});

describe("confirmed configuration lifecycle", () => {
  it("preserves model-only updates and clears unsettled or empty snapshots", () => {
    const store = makeStore();
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;
    const planOption = {
      type: "select",
      id: collaborationModeId,
      name: collaborationModeName,
      current_value: "plan",
      options: [],
    };

    handler(
      makeMessage(
        makePayload(providerModelId, {
          config_options_settled: true,
          config_options: [planOption],
        }),
      ),
    );
    expect(store.getState().sessionModels.bySessionId["session-1"].confirmedConfigOptions).toEqual({
      [collaborationModeId]: "plan",
    });

    handler(
      makeMessage(
        makePayload("next-model", { config_options_settled: undefined, config_options: [] }),
      ),
    );
    expect(store.getState().sessionModels.bySessionId["session-1"].confirmedConfigOptions).toEqual({
      [collaborationModeId]: "plan",
    });

    handler(makeMessage(makePayload("", { config_options_settled: false, config_options: [] })));
    expect(
      store.getState().sessionModels.bySessionId["session-1"].confirmedConfigOptions,
    ).toBeUndefined();

    handler(makeMessage(makePayload("", { config_options_settled: true, config_options: [] })));
    expect(store.getState().sessionModels.bySessionId["session-1"].confirmedConfigOptions).toEqual(
      {},
    );
  });
});

describe("live provider updates", () => {
  it("refreshes an open plan menu from post-startup provider updates", () => {
    const store = makeStore();
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;
    const planCommand = {
      name: "plan",
      description: "Set plan mode",
      action: {
        kind: "set_config_option",
        config_id: collaborationModeId,
        value: "plan",
        reset_value: "default",
      },
    };
    const menuCommand = () =>
      mapAvailableCommandToSlashCommand(
        planCommand,
        store.getState().sessionModels.bySessionId["session-1"]?.confirmedConfigOptions,
      );

    handler(
      makeMessage(
        makePayload(providerModelId, {
          config_options_settled: true,
          agent_execution_id: executionId,
          config_options: [modeOption("default")],
        }),
      ),
    );
    expect(menuCommand().modeState).toBe("default");

    for (const value of ["plan", "default"]) {
      const providerUpdate = {
        ...makePayload(providerModelId, { config_options: [modeOption(value)] }),
        config_options_source: "provider_update",
        agent_execution_id: executionId,
      } as SessionModelsPayload;
      handler(makeMessage(providerUpdate));
      expect(menuCommand().modeState).toBe(value === "plan" ? "active" : "default");
    }
  });

  it("preserves confirmed values omitted from a partial provider update", () => {
    const store = makeStore();
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

    handler(
      makeMessage(
        makePayload(providerModelId, {
          config_options_settled: true,
          agent_execution_id: executionId,
          config_options: [modeOption("plan")],
        }),
      ),
    );

    handler(
      makeMessage(
        makePayload(providerModelId, {
          agent_execution_id: executionId,
          config_options_source: "provider_update",
          config_options: [
            {
              type: "select",
              id: "approval_policy",
              name: "Approval Policy",
              current_value: "on-request",
              options: [],
            },
          ],
        }),
      ),
    );

    expect(store.getState().sessionModels.bySessionId["session-1"].confirmedConfigOptions).toEqual({
      [collaborationModeId]: "plan",
      approval_policy: "on-request",
    });
  });
});

describe("provider execution ownership", () => {
  it("does not confirm a provider update from an unsettled new execution", () => {
    const store = makeStore({
      sessionAgentctl: {
        itemsBySessionId: {
          "session-1": { status: "starting", agentExecutionId: "execution-new" },
        },
      },
    });
    store.getState().sessionModels.bySessionId["session-1"] = {
      currentModelId: providerModelId,
      models: [],
      configOptions: [],
      confirmedConfigOptions: { [collaborationModeId]: "plan" },
      confirmedConfigOptionsExecutionId: "execution-old",
    };
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;

    handler(
      makeMessage(
        makePayload(providerModelId, {
          config_options: [
            {
              type: "select",
              id: collaborationModeId,
              name: collaborationModeName,
              current_value: "default",
              options: [],
            },
          ],
          config_options_source: "provider_update",
          agent_execution_id: "execution-new",
        }),
      ),
    );

    expect(
      store.getState().sessionModels.bySessionId["session-1"].confirmedConfigOptions,
    ).toBeUndefined();
  });
});

describe("session-specific confirmed configuration", () => {
  it("does not mix values between sessions", () => {
    const store = makeStore();
    const handler = registerSessionModelsHandlers(store)["session.models_updated"]!;
    handler(
      makeMessage(
        makePayload(providerModelId, {
          config_options_settled: true,
          config_options: [modeOption("plan")],
        }),
      ),
    );
    handler(
      makeMessage(
        makePayload(providerModelId, {
          session_id: "session-2",
          config_options_settled: true,
          config_options: [modeOption("default")],
        }),
      ),
    );

    expect(store.getState().sessionModels.bySessionId["session-1"].confirmedConfigOptions).toEqual({
      [collaborationModeId]: "plan",
    });
    expect(store.getState().sessionModels.bySessionId["session-2"].confirmedConfigOptions).toEqual({
      [collaborationModeId]: "default",
    });
  });
});
