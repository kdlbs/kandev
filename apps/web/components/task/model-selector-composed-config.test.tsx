import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ModelSelector } from "@/components/task/model-selector";
import {
  setSessionConfigOption,
  setSessionModel,
  type SetSessionConfigurationResponse,
} from "@/lib/api/domains/session-api";
import { ApiError } from "@/lib/api/client";
import type { SessionConfigurationFeedbackUpdate } from "@/lib/state/slices/session-runtime/session-configuration-feedback";

// eslint-disable-next-line max-lines-per-function -- One hoisted fixture graph keeps the live-session state and model catalog consistent.
const mocks = vi.hoisted(() => {
  const SESSION_ID = "session-composed";
  const EXECUTION_ID = "execution-1";
  const REPLACEMENT_EXECUTION_ID = "execution-2";
  const PAIR_B = "pair-b";
  const THINKING_OPTION = "thinking";
  const HIGH_OPTION = "high";
  const listeners = new Set<() => void>();
  const toast = vi.fn();
  const responseFor = (configId: string, value: string): SetSessionConfigurationResponse => ({
    ok: true,
    configuration_result: {
      status: "applied",
      config_id: configId,
      requested: value,
      effective: value,
      confirmed: true,
      execution_id: EXECUTION_ID,
    },
  });
  const option = (value: string, name: string) => ({ value, name });
  const modelOption = {
    type: "select",
    id: "model",
    name: "Model",
    currentValue: "pair-a",
    category: "model",
    options: [option("pair-a", "Balanced A"), option(PAIR_B, "Balanced B"), option("solo", "Solo")],
    catalog: {
      version: 1,
      nodes: [
        {
          kind: "group",
          id: "composed",
          label: "Composed",
          children: [
            { kind: "choice", value: "pair-a", label: "Balanced A" },
            { kind: "choice", value: PAIR_B, label: "Balanced B" },
          ],
        },
        {
          kind: "group",
          id: "plain",
          label: "Plain",
          children: [{ kind: "choice", value: "solo", label: "Solo" }],
        },
      ],
      compositions: [
        {
          id: "composed",
          group_id: "composed",
          label: "Composed",
          dimensions: [
            {
              id: "pairing",
              label: "Pairing",
              choices: [
                { value: "a", label: "Pairing A" },
                { value: "b", label: "Pairing B" },
              ],
            },
          ],
          bindings: [
            { values: { pairing: "a" }, model: "pair-a" },
            { values: { pairing: "b" }, model: PAIR_B },
          ],
        },
      ],
    },
  };
  const appState = {
    activeModel: { bySessionId: {} as Record<string, string> },
    sessionModels: {
      bySessionId: {
        [SESSION_ID]: {
          currentModelId: "pair-a",
          models: [
            { modelId: "pair-a", name: "Balanced A" },
            { modelId: PAIR_B, name: "Balanced B" },
            { modelId: "solo", name: "Solo" },
          ],
          configOptions: [
            modelOption,
            {
              type: "select",
              id: THINKING_OPTION,
              name: "Thinking",
              currentValue: "low",
              options: [option("low", "Low"), option(HIGH_OPTION, "High")],
            },
          ],
          configOptionsSettled: true,
          confirmedConfigOptionsExecutionId: EXECUTION_ID,
        },
      } as Record<string, unknown>,
    },
    sessionMode: { bySessionId: { [SESSION_ID]: { currentModeId: "build" } } },
    settingsAgents: { items: [] },
    agentProfiles: { items: [] },
    turns: { bySession: {} },
    quickChat: { sessions: [] },
    recordSessionConfigurationFeedback:
      vi.fn<(sessionId: string, update: SessionConfigurationFeedbackUpdate) => void>(),
    taskSessions: {
      items: {
        [SESSION_ID]: {
          agent_profile_id: "profile-1",
          agent_profile_snapshot: { agent_name: "devin-acp", config_options: { thinking: "low" } },
          agent_execution_id: EXECUTION_ID,
          metadata: { runtime_config: { config_options: { thinking: "low", model: "pair-a" } } },
        },
      },
    },
    setActiveModel: vi.fn((sessionId: string, modelId: string) => {
      appState.activeModel.bySessionId[sessionId] = modelId;
      for (const listener of listeners) listener();
    }),
    setSessionModels: vi.fn((sessionId: string, data: unknown) => {
      appState.sessionModels.bySessionId[sessionId] = data;
      for (const listener of listeners) listener();
    }),
  };
  const initialSession = JSON.parse(
    JSON.stringify(appState.sessionModels.bySessionId[SESSION_ID]),
  ) as Record<string, unknown>;
  const initialTaskSession = JSON.parse(
    JSON.stringify(appState.taskSessions.items[SESSION_ID]),
  ) as Record<string, unknown>;
  return {
    appState,
    listeners,
    toast,
    SESSION_ID,
    EXECUTION_ID,
    REPLACEMENT_EXECUTION_ID,
    PAIR_B,
    THINKING_OPTION,
    HIGH_OPTION,
    initialSession,
    initialTaskSession,
    responseFor,
  };
});

const SESSION_ID = mocks.SESSION_ID;
const { EXECUTION_ID, REPLACEMENT_EXECUTION_ID, PAIR_B, THINKING_OPTION, HIGH_OPTION } = mocks;
const PERSISTENCE_FAILED = "persistence_failed" as const;
const OPTION_REJECTION = "option rejected";
const SAVE_PERSISTENCE_ERROR = "failed to persist session runtime config";

vi.mock("@/components/state-provider", async () => {
  const { useEffect, useReducer } = await import("react");
  const store = { getState: () => mocks.appState };
  return {
    useAppStoreApi: () => store,
    useAppStore: (selector: (state: typeof mocks.appState) => unknown) => {
      const [, force] = useReducer((count: number) => count + 1, 0);
      useEffect(() => {
        mocks.listeners.add(force);
        return () => {
          mocks.listeners.delete(force);
        };
      }, [force]);
      return selector(mocks.appState);
    },
  };
});

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/hooks/domains/settings/use-available-agents", () => ({
  useAvailableAgents: () => ({ items: [] }),
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: vi.fn(),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  resolveSessionModelConfig: vi.fn(async (_sessionId: string, request: { model: string }) => ({
    agent_name: "devin-acp",
    model: request.model,
    status: "ok",
    config_options: [
      {
        type: "select",
        id: THINKING_OPTION,
        name: "Thinking",
        current_value: "low",
        options: [
          { value: "low", name: "Low" },
          { value: HIGH_OPTION, name: "High" },
        ],
      },
    ],
    error: null,
  })),
  setSessionConfigOption: vi.fn((_sessionId: string, configId: string, value: string) =>
    Promise.resolve(mocks.responseFor(configId, value)),
  ),
  setSessionMode: vi.fn(() => Promise.resolve({ ok: true })),
  setSessionModel: vi.fn((_sessionId: string, modelId: string) =>
    Promise.resolve(mocks.responseFor("model", modelId)),
  ),
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchDynamicModels: vi.fn(async () => ({
    agent_name: "devin-acp",
    status: "ok",
    models: [],
    modes: [],
    commands: [],
    error: null,
  })),
  resolveAgentModelConfig: vi.fn(async (_agentName: string, request: { model: string }) => ({
    agent_name: "devin-acp",
    model: request.model,
    status: "ok",
    config_options: [
      {
        type: "select",
        id: THINKING_OPTION,
        name: "Thinking",
        current_value: "low",
        options: [
          { value: "low", name: "Low" },
          { value: HIGH_OPTION, name: "High" },
        ],
      },
    ],
    error: null,
  })),
}));

beforeEach(() => {
  mocks.appState.recordSessionConfigurationFeedback.mockClear();
  mocks.appState.activeModel.bySessionId = {};
  (mocks.appState.taskSessions.items as Record<string, Record<string, unknown>>)[SESSION_ID] =
    JSON.parse(JSON.stringify(mocks.initialTaskSession)) as Record<string, unknown>;
  mocks.appState.sessionModels.bySessionId[SESSION_ID] = JSON.parse(
    JSON.stringify(mocks.initialSession),
  ) as Record<string, unknown>;
});

function recordedFeedback() {
  return mocks.appState.recordSessionConfigurationFeedback.mock.calls
    .map(([, update]) => update)
    .findLast((update) => update.entries.length > 0);
}

async function latestFeedback() {
  await waitFor(() => expect(recordedFeedback()).toBeDefined());
  return recordedFeedback()!;
}

function replaceSessionExecution(executionId: string, currentModelId: string) {
  const live = mocks.appState.sessionModels.bySessionId[SESSION_ID] as {
    configOptions: Array<{ id: string; currentValue?: string }>;
  } & Record<string, unknown>;
  mocks.appState.sessionModels.bySessionId[SESSION_ID] = {
    ...live,
    currentModelId,
    confirmedConfigOptionsExecutionId: executionId,
    configOptions: live.configOptions.map((option) =>
      option.id === "model" ? { ...option, currentValue: currentModelId } : option,
    ),
  };
  const taskSession = (
    mocks.appState.taskSessions.items as Record<string, Record<string, unknown>>
  )[SESSION_ID]!;
  (mocks.appState.taskSessions.items as Record<string, Record<string, unknown>>)[SESSION_ID] = {
    ...taskSession,
    agent_execution_id: executionId,
  };
  for (const listener of mocks.listeners) listener();
}

afterEach(() => {
  cleanup();
  vi.mocked(setSessionConfigOption).mockClear();
  vi.mocked(setSessionModel).mockClear();
  mocks.toast.mockClear();
  vi.mocked(setSessionConfigOption).mockImplementation((_sessionId, configId, value) =>
    Promise.resolve(mocks.responseFor(configId, value)),
  );
});

function openPicker() {
  const trigger = screen.getByRole("button", { name: "Session model settings" });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
}

async function applyComposedPairing() {
  openPicker();
  fireEvent.click(screen.getByTestId("model-catalog-composition-row-composed"));
  fireEvent.click(screen.getByTestId("model-composition-dimension-pairing"));
  fireEvent.click(screen.getByTestId("model-composition-choice-pairing-b"));
  await waitFor(() => expect(screen.getByTestId("model-provider-option-thinking")).toBeTruthy());
  fireEvent.click(screen.getByTestId("model-provider-option-select-thinking"));
  fireEvent.click(await screen.findByRole("option", { name: "High" }));
  fireEvent.click(screen.getByTestId("model-composition-apply"));
}

describe("live composed configuration apply", () => {
  it("confirms the model then the real provider options", async () => {
    render(
      <TooltipProvider>
        <ModelSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    await applyComposedPairing();

    await waitFor(() => expect(setSessionConfigOption).toHaveBeenCalledTimes(2));
    expect(vi.mocked(setSessionConfigOption).mock.calls[0]).toEqual([SESSION_ID, "model", PAIR_B]);
    expect(vi.mocked(setSessionConfigOption).mock.calls[1]).toEqual([
      SESSION_ID,
      THINKING_OPTION,
      HIGH_OPTION,
    ]);
  });

  it("stops dependent option writes when the base model is clamped", async () => {
    vi.mocked(setSessionConfigOption).mockImplementationOnce(
      async (_sessionId, configId, value) => ({
        ok: true,
        configuration_result: {
          status: "applied",
          config_id: configId,
          requested: value,
          effective: "solo",
          confirmed: true,
          execution_id: EXECUTION_ID,
        },
      }),
    );
    render(
      <TooltipProvider>
        <ModelSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    await applyComposedPairing();

    await waitFor(() => {
      expect(
        (
          mocks.appState.sessionModels.bySessionId[SESSION_ID] as {
            currentModelId: string;
          }
        ).currentModelId,
      ).toBe("solo");
    });
    const feedback = await latestFeedback();
    expect(feedback.entries[0]).toMatchObject({
      text: "Requested Model: Balanced B. The agent applied Model: Solo.",
    });
    expect(setSessionConfigOption).toHaveBeenCalledTimes(1);
  });
});

describe("composed configuration partial retries", () => {
  it("reports a partial application and retries without re-applying the model", async () => {
    const session = mocks.appState.sessionModels.bySessionId[SESSION_ID] as {
      configOptions: Array<{ id: string; currentValue?: string }>;
    };
    const thinking = session.configOptions.find((option) => option.id === THINKING_OPTION);
    if (!thinking) throw new Error("missing thinking config option");
    thinking.currentValue = "medium";

    vi.mocked(setSessionConfigOption)
      .mockRejectedValueOnce(
        new ApiError(SAVE_PERSISTENCE_ERROR, 500, {
          error: SAVE_PERSISTENCE_ERROR,
          configuration_result: {
            status: PERSISTENCE_FAILED,
            config_id: "model",
            requested: PAIR_B,
            effective: PAIR_B,
            confirmed: true,
            execution_id: EXECUTION_ID,
          },
        }),
      )
      .mockRejectedValueOnce(
        new ApiError(OPTION_REJECTION, 500, {
          error: OPTION_REJECTION,
          configuration_result: {
            status: "rejected",
            config_id: THINKING_OPTION,
            requested: HIGH_OPTION,
            confirmed: false,
            execution_id: EXECUTION_ID,
          },
        }),
      )
      .mockImplementationOnce(async (_sessionId, configId, value) => ({
        ...mocks.responseFor(configId, value),
        configuration_result: {
          status: "applied",
          config_id: configId,
          requested: value,
          effective: "low",
          confirmed: true,
          execution_id: EXECUTION_ID,
        },
      }));
    render(
      <TooltipProvider>
        <ModelSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    await applyComposedPairing();

    const partial = await latestFeedback();
    expect(
      (
        mocks.appState.sessionModels.bySessionId[SESSION_ID] as {
          currentModelId: string;
        }
      ).currentModelId,
    ).toBe(PAIR_B);
    expect(partial.entries[0]).toHaveProperty(
      "text",
      `Model: Balanced B applied, but it could not be saved. ${SAVE_PERSISTENCE_ERROR}`,
    );
    expect(partial.entries[1]).toHaveProperty(
      "text",
      `Could not apply Thinking: High. ${OPTION_REJECTION}`,
    );
    expect(partial.retry).toBeTruthy();
    expect(mocks.toast).not.toHaveBeenCalled();

    vi.mocked(setSessionConfigOption).mockClear();
    mocks.appState.recordSessionConfigurationFeedback.mockClear();
    await act(async () => {
      partial.retry?.run();
      await Promise.resolve();
    });
    await waitFor(() => expect(setSessionConfigOption).toHaveBeenCalledTimes(1));
    expect(vi.mocked(setSessionConfigOption).mock.calls[0]).toEqual([
      SESSION_ID,
      THINKING_OPTION,
      HIGH_OPTION,
      PAIR_B,
    ]);
    const retried = await latestFeedback();
    expect(retried.entries).toHaveLength(1);
    expect(retried.entries[0]).toHaveProperty("text", expect.stringContaining("Thinking: Low"));
    expect(
      (
        mocks.appState.sessionModels.bySessionId[SESSION_ID] as {
          configOptions: Array<{ id: string; currentValue: string }>;
        }
      ).configOptions.find((option) => option.id === THINKING_OPTION)?.currentValue,
    ).toBe("low");
  });
});

describe("composed configuration retry failures", () => {
  it("keeps the rejection reason after a second retry failure", async () => {
    const rejectedOption = (message: string) =>
      new ApiError(message, 500, {
        error: message,
        configuration_result: {
          status: "rejected",
          config_id: THINKING_OPTION,
          requested: HIGH_OPTION,
          confirmed: false,
          execution_id: EXECUTION_ID,
        },
      });
    vi.mocked(setSessionConfigOption)
      .mockImplementationOnce((_sessionId, configId, value) =>
        Promise.resolve(mocks.responseFor(configId, value)),
      )
      .mockRejectedValueOnce(rejectedOption("first option rejection"))
      .mockRejectedValueOnce(rejectedOption("retry option rejection"));
    render(
      <TooltipProvider>
        <ModelSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    await applyComposedPairing();
    const initial = await latestFeedback();
    expect(initial.retry).toBeTruthy();
    mocks.appState.recordSessionConfigurationFeedback.mockClear();
    await act(async () => initial.retry?.run());

    const feedback = await latestFeedback();
    expect(feedback.entries[0]).toHaveProperty(
      "text",
      expect.stringContaining("retry option rejection"),
    );
    expect(feedback.retry).toBeTruthy();
  });
});

describe("composed configuration request fencing", () => {
  it("does not confirm a superseded application", async () => {
    let resolveModel: ((value: SetSessionConfigurationResponse) => void) | undefined;
    vi.mocked(setSessionConfigOption)
      .mockImplementationOnce(
        () =>
          new Promise<SetSessionConfigurationResponse>((resolve) => {
            resolveModel = resolve;
          }),
      )
      .mockImplementation(() => Promise.resolve({ ok: true }));
    render(
      <TooltipProvider>
        <ModelSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    await applyComposedPairing();
    await waitFor(() =>
      expect(setSessionConfigOption).toHaveBeenCalledWith(SESSION_ID, "model", PAIR_B),
    );
    vi.mocked(setSessionConfigOption).mockClear();

    // The picker stays open while extra options exist; the completed view
    // returns to the list. A newer ordinary selection supersedes the in-flight apply.
    await waitFor(() => expect(screen.queryByTestId("model-composition-apply")).toBeNull());
    fireEvent.click(await screen.findByRole("option", { name: /Solo/ }));
    await waitFor(() =>
      expect(setSessionConfigOption).toHaveBeenCalledWith(SESSION_ID, "model", "solo"),
    );

    await act(async () => {
      resolveModel?.({ ok: true });
      await Promise.resolve();
    });
    expect(setSessionConfigOption).not.toHaveBeenCalledWith(
      SESSION_ID,
      THINKING_OPTION,
      HIGH_OPTION,
    );
    expect(mocks.toast).not.toHaveBeenCalled();
  });
});

describe("composed configuration execution fencing", () => {
  it("does not write an old execution's successful model result over its replacement", async () => {
    let resolveOld!: (value: SetSessionConfigurationResponse) => void;
    vi.mocked(setSessionConfigOption).mockImplementationOnce(
      () =>
        new Promise<SetSessionConfigurationResponse>((resolve) => {
          resolveOld = resolve;
        }),
    );
    render(
      <TooltipProvider>
        <ModelSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    openPicker();
    fireEvent.click(await screen.findByRole("option", { name: /Solo/ }));
    await waitFor(() =>
      expect(setSessionConfigOption).toHaveBeenCalledWith(SESSION_ID, "model", "solo"),
    );
    replaceSessionExecution(REPLACEMENT_EXECUTION_ID, PAIR_B);

    await act(async () => {
      resolveOld({
        ok: true,
        configuration_result: {
          status: "applied",
          config_id: "model",
          requested: "solo",
          effective: "solo",
          confirmed: true,
          execution_id: EXECUTION_ID,
        },
      });
      await Promise.resolve();
    });

    expect(
      (
        mocks.appState.sessionModels.bySessionId[SESSION_ID] as {
          currentModelId: string;
        }
      ).currentModelId,
    ).toBe(PAIR_B);
    expect(recordedFeedback()).toBeUndefined();
  });

  it("does not restore an old execution snapshot after a delayed failure", async () => {
    let rejectOld!: (reason: Error) => void;
    vi.mocked(setSessionConfigOption).mockImplementationOnce(
      () =>
        new Promise<SetSessionConfigurationResponse>((_resolve, reject) => {
          rejectOld = reject;
        }),
    );
    render(
      <TooltipProvider>
        <ModelSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    openPicker();
    fireEvent.click(await screen.findByRole("option", { name: /Solo/ }));
    await waitFor(() =>
      expect(setSessionConfigOption).toHaveBeenCalledWith(SESSION_ID, "model", "solo"),
    );
    replaceSessionExecution(REPLACEMENT_EXECUTION_ID, PAIR_B);

    await act(async () => {
      rejectOld(new Error("old execution timed out"));
      await Promise.resolve();
    });

    expect(
      (
        mocks.appState.sessionModels.bySessionId[SESSION_ID] as {
          currentModelId: string;
        }
      ).currentModelId,
    ).toBe(PAIR_B);
    expect(recordedFeedback()).toBeUndefined();
  });
});
