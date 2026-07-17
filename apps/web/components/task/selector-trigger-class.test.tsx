import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ModelSelector } from "@/components/task/model-selector";
import { ModeSelector } from "@/components/task/mode-selector";

const mocks = vi.hoisted(() => {
  const appState = {
    activeModel: { bySessionId: {} },
    sessionModels: {
      bySessionId: {
        "session-1": {
          currentModelId: "gpt-5.5",
          models: [{ modelId: "gpt-5.5", name: "GPT-5.5" }],
          configOptions: [
            {
              type: "select",
              id: "reasoning_effort",
              name: "Reasoning Effort",
              currentValue: "low",
              options: [{ value: "low", name: "Low" }],
            },
            {
              type: "select",
              id: "fast_mode",
              name: "Fast Mode",
              currentValue: "off",
              options: [{ value: "off", name: "Off" }],
            },
          ],
          configBaseline: { reasoning_effort: "high", fast_mode: "off" },
        },
      },
    },
    sessionMode: {
      bySessionId: {
        "session-1": {
          currentModeId: "full-access",
          availableModes: [
            { id: "full-access", name: "Full access" },
            { id: "read-only", name: "Read only" },
          ],
        },
      },
    },
    settingsAgents: { items: [] },
    taskSessions: {
      items: {
        "session-1": {
          agent_profile_id: "profile-1",
          agent_profile_snapshot: {},
        },
      },
    },
    setActiveModel: vi.fn(),
    setSessionModels: vi.fn(),
  };

  return {
    appState,
    storeSelections: [] as unknown[],
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.appState) => unknown) => {
    const result = selector(mocks.appState);
    mocks.storeSelections.push(result);
    return result;
  },
}));

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
  setSessionConfigOption: vi.fn(),
  setSessionMode: vi.fn(),
  setSessionModel: vi.fn(),
}));

afterEach(() => {
  cleanup();
  mocks.storeSelections.length = 0;
});

describe("task selector trigger styling", () => {
  it("forwards custom trigger classes to the model selector trigger", () => {
    render(
      <TooltipProvider>
        <ModelSelector sessionId="session-1" triggerClassName="max-w-model" />
      </TooltipProvider>,
    );

    expect(screen.getByRole("button", { name: "Session model settings" }).className).toContain(
      "max-w-model",
    );
  });

  it("opts the task model selector into its changed-values summary", () => {
    render(
      <TooltipProvider>
        <ModelSelector sessionId="session-1" />
      </TooltipProvider>,
    );

    expect(screen.getByRole("button", { name: "Session model settings" }).textContent).toBe(
      "GPT-5.5 / Low",
    );
  });

  it("subscribes only to the active session model entry", () => {
    render(
      <TooltipProvider>
        <ModelSelector sessionId="session-1" />
      </TooltipProvider>,
    );

    expect(mocks.storeSelections).toContain(mocks.appState.sessionModels.bySessionId["session-1"]);
    expect(mocks.storeSelections).not.toContain(mocks.appState.sessionModels.bySessionId);
  });

  it("forwards custom trigger classes to the mode selector trigger", () => {
    render(
      <TooltipProvider>
        <ModeSelector sessionId="session-1" triggerClassName="max-w-mode" />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("session-mode-selector").className).toContain("max-w-mode");
  });
});
