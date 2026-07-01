import { cleanup, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ReactElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ModelSelector } from "@/components/task/model-selector";
import { ModeSelector } from "@/components/task/mode-selector";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";

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
  useSettingsData: () => ({
    settingsAgents: [],
    agentProfiles: [],
    executors: [],
    availableAgents: [],
    availableTools: [],
    settingsData: {
      agentsLoaded: true,
      capabilitiesLoaded: true,
      executorsLoaded: true,
    },
  }),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  setSessionConfigOption: vi.fn(),
  setSessionMode: vi.fn(),
  setSessionModel: vi.fn(),
}));

function renderWithQuery(ui: ReactElement) {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(
    qk.sessionRuntime.models("session-1"),
    mocks.appState.sessionModels.bySessionId["session-1"],
  );
  queryClient.setQueryData(
    qk.sessionRuntime.mode("session-1"),
    mocks.appState.sessionMode.bySessionId["session-1"],
  );
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);
}

describe("task selector trigger styling", () => {
  afterEach(() => {
    cleanup();
  });

  it("forwards custom trigger classes to the model selector trigger", () => {
    renderWithQuery(<ModelSelector sessionId="session-1" triggerClassName="max-w-model" />);

    expect(screen.getByRole("button", { name: "Session model settings" }).className).toContain(
      "max-w-model",
    );
  });

  it("forwards custom trigger classes to the mode selector trigger", () => {
    renderWithQuery(
      <TooltipProvider>
        <ModeSelector sessionId="session-1" triggerClassName="max-w-mode" />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("session-mode-selector").className).toContain("max-w-mode");
  });
});
