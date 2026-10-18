import { act, cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { TaskLspLanguageSnapshot } from "@/lib/types/http-lsp";
import { AppStatusBar } from "./app-status-bar";
import { APP_STATUS_LSP_ID } from "./app-status-bar-order";

const TEST_TIME = "2026-08-05T12:00:00Z";

const kotlin: TaskLspLanguageSnapshot = {
  task_id: "task-1",
  language: "kotlin",
  policy: "keep_warm",
  detected: true,
  detection_state: "complete",
  detection_truncated: false,
  phase: "initializing",
  generation: 2,
  revision: 4,
  last_transition_at: TEST_TIME,
  last_action: "start",
  last_action_at: TEST_TIME,
  last_initiator: "user",
  restart_required: false,
  created_at: TEST_TIME,
  updated_at: TEST_TIME,
  effective_policy: "keep_warm",
  activity: "server_work",
  progress: [
    {
      token: "gradle",
      title: "Importing Kotlin project",
      started_at: TEST_TIME,
    },
  ],
};

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: true, isMobile: false }),
}));

vi.mock("@/hooks/domains/lsp/use-task-lsp", () => ({
  useTaskLsp: () => ({
    languages: [kotlin],
    pending: {},
    loaded: true,
    loading: false,
    error: null,
    capacity: { active: 1, queued: 0, limit: 4 },
    start: vi.fn(),
    stop: vi.fn(),
    restart: vi.fn(),
    setPolicy: vi.fn(),
  }),
}));

afterEach(() => {
  cleanup();
  useDockviewStore.setState({
    activeFilePath: null,
    activeFileRepo: null,
    activePanelComponent: null,
    openFiles: new Map(),
  });
});

function renderBar({ taskId = "task-1" }: { taskId?: string | null } = {}) {
  return render(
    <StateProvider
      initialState={{
        userSettings: {
          ...defaultSettingsState.userSettings,
          appStatusBarEnabled: true,
          lspStatusLocation: "toolbar",
        },
      }}
    >
      <TooltipProvider>
        <AppStatusBar
          pathname="/tasks/task-1"
          activeWorkspaceId="workspace-1"
          activeTaskId={taskId}
          activeSessionId="session-1"
          density="full"
        />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("task-scoped LSP status item integration", () => {
  it("stays visible while an unsupported file or non-editor panel is active", () => {
    renderBar();
    const lspItem = () => document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`);

    expect(lspItem()).toBeTruthy();
    expect(screen.getByTestId("app-status-lsp").textContent).toContain("Kotlin");
    expect(screen.getByTestId("app-status-lsp").textContent).toContain("Importing Kotlin project");

    act(() => {
      useDockviewStore.setState({
        activeFilePath: "README.md",
        activeFileRepo: "app",
        activePanelComponent: "file-editor",
      });
    });
    expect(lspItem()).toBeTruthy();

    act(() => {
      useDockviewStore.setState({ activePanelComponent: "terminal" });
    });
    expect(lspItem()).toBeTruthy();
  });

  it("ignores the retired per-user placement setting", () => {
    renderBar();
    expect(document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`)).toBeTruthy();
  });

  it("does not render task controls without an active task", () => {
    renderBar({ taskId: null });
    expect(document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`)).toBeNull();
  });
});
