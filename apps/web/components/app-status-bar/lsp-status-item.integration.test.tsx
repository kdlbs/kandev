import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultFeaturesState } from "@/lib/state/slices/features/features-slice";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { AppStatusBar } from "./app-status-bar";
import { APP_STATUS_LSP_ID } from "./app-status-bar-order";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isFinePointer: true,
    isMobile: false,
  }),
}));

vi.mock("@/hooks/use-lsp", () => ({
  useLspStatus: () => ({
    status: { state: "disabled" },
    progress: {
      initializingSince: null,
      active: [],
      completed: null,
      hasReportedProgress: false,
    },
    toggle: vi.fn(),
  }),
}));

afterEach(() => {
  cleanup();
  useDockviewStore.setState({ activeFilePath: null, activeFileRepo: null });
});

function renderBar(location: "toolbar" | "status_bar") {
  return render(
    <StateProvider
      initialState={{
        features: { ...defaultFeaturesState.features, appStatusBar: true },
        userSettings: {
          ...defaultSettingsState.userSettings,
          lspStatusLocation: location,
        },
      }}
    >
      <TooltipProvider>
        <AppStatusBar
          pathname="/tasks/task-1"
          activeWorkspaceId="workspace-1"
          activeTaskId="task-1"
          activeSessionId="session-1"
          density="full"
        />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("active-editor LSP status item integration", () => {
  it("renders the supported active file only for status-bar placement", () => {
    useDockviewStore.setState({ activeFilePath: "src/Main.kt", activeFileRepo: "app" });
    renderBar("status_bar");

    expect(document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`)).toBeTruthy();
    expect(screen.getByTestId("app-status-lsp").textContent).toContain("Kotlin");
  });

  it("hides for toolbar placement and when the active panel becomes unsupported", () => {
    useDockviewStore.setState({ activeFilePath: "src/Main.kt", activeFileRepo: "app" });
    const rendered = renderBar("toolbar");
    expect(document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`)).toBeNull();

    rendered.unmount();
    useDockviewStore.setState({ activeFilePath: "README.md", activeFileRepo: null });
    renderBar("status_bar");
    expect(document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`)).toBeNull();
  });
});
