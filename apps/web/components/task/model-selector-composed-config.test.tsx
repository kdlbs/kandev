import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ModelSelector } from "@/components/task/model-selector";
import { setSessionConfigOption, setSessionModel } from "@/lib/api/domains/session-api";

// eslint-disable-next-line max-lines-per-function -- One hoisted fixture graph keeps the live-session state and model catalog consistent.
const mocks = vi.hoisted(() => {
  const SESSION_ID = "session-composed";
  const listeners = new Set<() => void>();
  const toast = vi.fn();
  const option = (value: string, name: string) => ({ value, name });
  const modelOption = {
    type: "select",
    id: "model",
    name: "Model",
    currentValue: "pair-a",
    category: "model",
    options: [
      option("pair-a", "Balanced A"),
      option("pair-b", "Balanced B"),
      option("solo", "Solo"),
    ],
    catalog: {
      version: 1,
      nodes: [
        {
          kind: "group",
          id: "composed",
          label: "Composed",
          children: [
            { kind: "choice", value: "pair-a", label: "Balanced A" },
            { kind: "choice", value: "pair-b", label: "Balanced B" },
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
            { values: { pairing: "b" }, model: "pair-b" },
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
            { modelId: "pair-b", name: "Balanced B" },
            { modelId: "solo", name: "Solo" },
          ],
          configOptions: [
            modelOption,
            {
              type: "select",
              id: "thinking",
              name: "Thinking",
              currentValue: "low",
              options: [option("low", "Low"), option("high", "High")],
            },
          ],
          configOptionsSettled: true,
          confirmedConfigOptionsExecutionId: "execution-1",
        },
      } as Record<string, unknown>,
    },
    sessionMode: { bySessionId: { [SESSION_ID]: { currentModeId: "build" } } },
    settingsAgents: { items: [] },
    agentProfiles: { items: [] },
    taskSessions: {
      items: {
        [SESSION_ID]: {
          agent_profile_id: "profile-1",
          agent_profile_snapshot: { agent_name: "devin-acp", config_options: { thinking: "low" } },
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
  return { appState, listeners, toast, SESSION_ID, initialSession };
});

const SESSION_ID = mocks.SESSION_ID;

vi.mock("@/components/state-provider", async () => {
  const { useEffect, useReducer } = await import("react");
  return {
    useAppStoreApi: () => ({ getState: () => mocks.appState }),
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
        id: "thinking",
        name: "Thinking",
        current_value: "low",
        options: [
          { value: "low", name: "Low" },
          { value: "high", name: "High" },
        ],
      },
    ],
    error: null,
  })),
  setSessionConfigOption: vi.fn(() => Promise.resolve({ ok: true })),
  setSessionMode: vi.fn(() => Promise.resolve({ ok: true })),
  setSessionModel: vi.fn(() => Promise.resolve({ ok: true })),
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
        id: "thinking",
        name: "Thinking",
        current_value: "low",
        options: [
          { value: "low", name: "Low" },
          { value: "high", name: "High" },
        ],
      },
    ],
    error: null,
  })),
}));

beforeEach(() => {
  mocks.appState.activeModel.bySessionId = {};
  mocks.appState.sessionModels.bySessionId[SESSION_ID] = JSON.parse(
    JSON.stringify(mocks.initialSession),
  ) as Record<string, unknown>;
});

afterEach(() => {
  cleanup();
  vi.mocked(setSessionConfigOption).mockClear();
  vi.mocked(setSessionModel).mockClear();
  mocks.toast.mockClear();
  vi.mocked(setSessionConfigOption).mockImplementation(() => Promise.resolve({ ok: true }));
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
    expect(vi.mocked(setSessionConfigOption).mock.calls[0]).toEqual([
      SESSION_ID,
      "model",
      "pair-b",
    ]);
    expect(vi.mocked(setSessionConfigOption).mock.calls[1]).toEqual([
      SESSION_ID,
      "thinking",
      "high",
    ]);
  });

  it("reports a partial application and retries without re-applying the model", async () => {
    vi.mocked(setSessionConfigOption)
      .mockImplementationOnce(() => Promise.resolve({ ok: true }))
      .mockRejectedValueOnce(new Error("option rejected"))
      .mockImplementation(() => Promise.resolve({ ok: true }));
    render(
      <TooltipProvider>
        <ModelSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    await applyComposedPairing();

    await waitFor(() => expect(mocks.toast).toHaveBeenCalled());
    const partial = mocks.toast.mock.calls.at(-1)?.[0] as {
      title: string;
      action?: { label: string; onClick: () => void };
    };
    expect(partial.title).toContain("An option was rejected");
    expect(partial.action).toBeDefined();

    vi.mocked(setSessionConfigOption).mockClear();
    await act(async () => {
      partial.action?.onClick();
      await Promise.resolve();
    });
    await waitFor(() => expect(setSessionConfigOption).toHaveBeenCalledTimes(1));
    expect(vi.mocked(setSessionConfigOption).mock.calls[0]?.[1]).toBe("thinking");
  });

  it("does not confirm a superseded application", async () => {
    let resolveModel: ((value: { ok: boolean }) => void) | undefined;
    vi.mocked(setSessionConfigOption)
      .mockImplementationOnce(
        () =>
          new Promise<{ ok: boolean }>((resolve) => {
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
      expect(setSessionConfigOption).toHaveBeenCalledWith(SESSION_ID, "model", "pair-b"),
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
    expect(setSessionConfigOption).not.toHaveBeenCalledWith(SESSION_ID, "thinking", "high");
    expect(mocks.toast).not.toHaveBeenCalled();
  });
});
