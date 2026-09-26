import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ModelSelector } from "@/components/task/model-selector";
import { ModeSelector } from "@/components/task/mode-selector";

const mocks = vi.hoisted(() => {
  const sessionId = "session-1" as const;
  const appState = {
    responsive: { isMobile: false, isFinePointer: true },
    activeModel: { bySessionId: {} },
    sessionModels: {
      bySessionId: {
        [sessionId]: {
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
        [sessionId]: {
          currentModeId: "full-access",
          availableModes: [
            { id: "read-only", name: "Read only" },
            { id: "full-access", name: "Full access" },
          ],
          requestedModeId: undefined as string | undefined,
        },
      },
    },
    settingsAgents: { items: [] },
    taskSessions: {
      items: {
        [sessionId]: {
          agent_profile_id: "profile-1",
          agent_profile_snapshot: {},
        },
        "session-pending": {
          agent_profile_id: "profile-pending",
          agent_profile_snapshot: {
            model: "gpt-5.6-sol",
            config_options: { reasoning_effort: "high" },
          },
        },
      },
    },
    setActiveModel: vi.fn(),
    setSessionModels: vi.fn(),
  };

  return {
    appState,
    sessionId,
    storeSelections: [] as unknown[],
    setSessionMode: vi.fn().mockResolvedValue(undefined),
  };
});

const SESSION_ID = mocks.sessionId;
const MODE_SELECTOR_TEST_ID = "session-mode-selector";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.appState) => unknown) => {
    const result = selector(mocks.appState);
    mocks.storeSelections.push(result);
    return result;
  },
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isMobile: mocks.appState.responsive.isMobile,
    isFinePointer: mocks.appState.responsive.isFinePointer,
  }),
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
  setSessionMode: mocks.setSessionMode,
  setSessionModel: vi.fn(),
}));

afterEach(() => {
  cleanup();
  mocks.storeSelections.length = 0;
  mocks.appState.responsive.isMobile = false;
  mocks.appState.responsive.isFinePointer = true;
  mocks.appState.sessionMode.bySessionId[SESSION_ID].requestedModeId = undefined;
  mocks.setSessionMode.mockClear();
});

describe("task selector trigger styling", () => {
  it("forwards custom trigger classes to the model selector trigger", () => {
    render(
      <TooltipProvider>
        <ModelSelector sessionId={SESSION_ID} triggerClassName="max-w-model" />
      </TooltipProvider>,
    );

    expect(screen.getByRole("button", { name: "Session model settings" }).className).toContain(
      "max-w-model",
    );
  });

  it("opts the task model selector into its changed-values summary", () => {
    render(
      <TooltipProvider>
        <ModelSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    expect(screen.getByRole("button", { name: "Session model settings" }).textContent).toBe(
      "GPT-5.5 / Low",
    );
  });

  it("subscribes only to the active session model entry", () => {
    render(
      <TooltipProvider>
        <ModelSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    expect(mocks.storeSelections).toContain(mocks.appState.sessionModels.bySessionId[SESSION_ID]);
    expect(mocks.storeSelections).not.toContain(mocks.appState.sessionModels.bySessionId);
  });

  it("waits for dynamic session config before rendering a partial label", () => {
    render(
      <TooltipProvider>
        <ModelSelector sessionId="session-pending" />
      </TooltipProvider>,
    );

    expect(screen.queryByRole("button", { name: "Session model settings" })).toBeNull();
  });

  it("forwards custom trigger classes to the mode selector trigger", () => {
    render(
      <TooltipProvider>
        <ModeSelector sessionId={SESSION_ID} triggerClassName="max-w-mode" />
      </TooltipProvider>,
    );

    expect(screen.getByTestId(MODE_SELECTOR_TEST_ID).className).toContain("max-w-mode");
  });

  it("puts the current mode first with a persistent selected surface", () => {
    render(
      <TooltipProvider>
        <ModeSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    const trigger = screen.getByTestId(MODE_SELECTOR_TEST_ID);
    fireEvent.pointerDown(trigger);
    fireEvent.click(trigger);

    const modes = screen.getAllByRole("menuitem");
    expect(modes.map((mode) => mode.textContent?.trim())).toEqual(["Full access", "Read only"]);
    expect(modes[0].className).toContain("bg-card");
    expect(modes[0].className).toContain("border-primary/50");
  });
});

describe("mobile mode selector", () => {
  it("shows the mode mismatch and choices in the phone picker and returns focus on close", async () => {
    mocks.appState.responsive.isMobile = true;
    mocks.appState.sessionMode.bySessionId[SESSION_ID].requestedModeId = "read-only";
    render(
      <TooltipProvider>
        <ModeSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    const trigger = screen.getByTestId("session-mode-selector");
    fireEvent.click(trigger);

    const picker = screen.getByRole("dialog");
    expect(picker.textContent).toContain("Requested Read only, running in Full access.");
    expect(picker.textContent).toContain("Read only");
    expect(picker.textContent).toContain("Full access");

    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() => expect(picker.getAttribute("data-state")).toBe("closed"));
  });

  it("uses the shared mode action from the phone picker", () => {
    mocks.appState.responsive.isMobile = true;
    render(
      <TooltipProvider>
        <ModeSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByTestId(MODE_SELECTOR_TEST_ID));
    fireEvent.click(screen.getByTestId("session-mode-option-read-only"));

    expect(mocks.setSessionMode).toHaveBeenCalledWith(SESSION_ID, "read-only");
    expect(screen.getByRole("dialog").getAttribute("data-state")).toBe("closed");
  });

  it("uses the phone picker on coarse-pointer non-phone viewports", () => {
    mocks.appState.responsive.isFinePointer = false;
    render(
      <TooltipProvider>
        <ModeSelector sessionId={SESSION_ID} />
      </TooltipProvider>,
    );

    const trigger = screen.getByTestId(MODE_SELECTOR_TEST_ID);
    expect(trigger.className).toContain("h-11");
    fireEvent.click(trigger);
    expect(screen.getByRole("dialog")).toBeTruthy();
  });
});
