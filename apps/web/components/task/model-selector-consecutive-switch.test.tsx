import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ModelSelector } from "@/components/task/model-selector";
import { setSessionConfigOption } from "@/lib/api/domains/session-api";

const mocks = vi.hoisted(() => {
  const SESSION_ID = "session-1";
  const OPUS = "opus[1m]";
  const listeners = new Set<() => void>();
  const modelOptions = [
    { value: "opus", name: "Opus" },
    { value: "opus[1m]", name: "Opus 1M" },
    { value: "sonnet", name: "Sonnet" },
    { value: "claude-fable-5", name: "Fable" },
    { value: "claude-fable-5[1m]", name: "Fable 1M" },
  ];
  const appState = {
    activeModel: { bySessionId: {} as Record<string, string> },
    sessionModels: {
      bySessionId: {
        [SESSION_ID]: {
          currentModelId: OPUS,
          models: modelOptions.map((option) => ({
            modelId: option.value,
            name: option.name,
          })),
          configOptions: [
            {
              type: "select",
              id: "mode",
              name: "Mode",
              currentValue: "agent",
              options: [{ value: "agent", name: "Agent" }],
            },
            {
              type: "select",
              id: "model",
              name: "Model",
              currentValue: OPUS,
              category: "model",
              options: modelOptions,
            },
            {
              type: "select",
              id: "effort",
              name: "Effort",
              currentValue: "high",
              options: [
                { value: "high", name: "High" },
                { value: "medium", name: "Medium" },
              ],
            },
            {
              type: "select",
              id: "fast",
              name: "Fast",
              currentValue: "false",
              options: [
                { value: "false", name: "Off" },
                { value: "true", name: "On" },
              ],
            },
          ],
          configOptionsSettled: true,
          configBaseline: { effort: "high", fast: "false", mode: "agent" },
        },
      } as Record<string, unknown>,
    },
    settingsAgents: { items: [] },
    taskSessions: {
      items: {
        [SESSION_ID]: {
          agent_profile_id: "profile-1",
          agent_profile_snapshot: { config_options: { fast: "false" } },
          metadata: {
            runtime_config: {
              config_options: { effort: "high", fast: "false", mode: "agent", model: OPUS },
            },
          },
        },
      },
    },
    setActiveModel: vi.fn((sessionId: string, modelId: string) => {
      appState.activeModel.bySessionId[sessionId] = modelId;
      notify();
    }),
    setSessionModels: vi.fn((sessionId: string, data: unknown) => {
      appState.sessionModels.bySessionId[sessionId] = data;
      notify();
    }),
  };
  function notify() {
    for (const listener of listeners) listener();
  }
  return { appState, listeners, modelOptions, SESSION_ID };
});

const SESSION_ID = mocks.SESSION_ID;
const OPUS = "opus[1m]";
const FABLE = "claude-fable-5[1m]";

vi.mock("@/components/state-provider", async () => {
  const { useEffect, useReducer } = await import("react");
  return {
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
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/hooks/domains/settings/use-available-agents", () => ({
  useAvailableAgents: () => ({ items: [] }),
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: vi.fn(),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  setSessionConfigOption: vi.fn(() => Promise.resolve({ ok: true })),
  setSessionMode: vi.fn(() => Promise.resolve({ ok: true })),
  setSessionModel: vi.fn(() => Promise.resolve({ ok: true })),
}));

afterEach(() => {
  cleanup();
  vi.mocked(setSessionConfigOption).mockClear();
});

function pickModel(label: string) {
  const trigger = screen.getByRole("button", { name: "Session model settings" });
  // Picking a model leaves the picker open when the agent advertises extra
  // config options, so only toggle it when it is actually closed.
  if (trigger.getAttribute("aria-expanded") !== "true") {
    fireEvent.pointerDown(trigger);
    fireEvent.click(trigger);
  }
  fireEvent.click(screen.getByRole("option", { name: new RegExp(label) }));
}

// Mirrors the convergence event the backend emits after a model switch: the
// provider re-advertises the option catalog for the newly selected model, which
// for Fable no longer carries the Opus-only "fast" toggle.
function applyConvergenceEvent(modelId: string, optionIds: string[]) {
  const previous = mocks.appState.sessionModels.bySessionId[SESSION_ID] as {
    configOptions: { id: string; currentValue: string }[];
  };
  act(() => {
    mocks.appState.setSessionModels(SESSION_ID, {
      ...previous,
      currentModelId: modelId,
      configOptions: previous.configOptions
        .filter((option) => optionIds.includes(option.id))
        .map((option) => (option.id === "model" ? { ...option, currentValue: modelId } : option)),
    });
  });
}

describe("consecutive session model switches", () => {
  it("sends every switch to the backend, not just the first", () => {
    render(
      <TooltipProvider>
        <ModelSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    pickModel("Fable 1M");
    expect(setSessionConfigOption).toHaveBeenNthCalledWith(1, SESSION_ID, "model", FABLE);
    applyConvergenceEvent(FABLE, ["mode", "model", "effort"]);

    pickModel("Opus 1M");
    expect(setSessionConfigOption).toHaveBeenNthCalledWith(2, SESSION_ID, "model", OPUS);
  });
});
