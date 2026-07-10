import { render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { describe, expect, it, vi } from "vitest";

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
          configOptions: [],
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
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.appState) => unknown) => selector(mocks.appState),
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

describe("task selector trigger styling", () => {
  it("forwards custom trigger classes to the model selector trigger", () => {
    render(<ModelSelector sessionId="session-1" triggerClassName="max-w-model" />);

    expect(screen.getByRole("button", { name: "Session model settings" }).className).toContain(
      "max-w-model",
    );
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
