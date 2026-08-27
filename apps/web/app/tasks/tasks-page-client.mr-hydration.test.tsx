import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";

// AC12: TasksPageClient must hydrate both change-request providers
// unconditionally, never gated on tasksListShowDetails. Every direct import
// of TasksPageClient is mocked so this test asserts only the wiring, not the behaviour
// of the many unrelated hooks/components the page also renders (per the
// plan's U9 guidance: mock rather than exercise the whole page).
const useWorkspaceMRsMock = vi.fn();
const useWorkspacePRsMock = vi.fn();

vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useWorkspaceMRs: (workspaceId: string | null) => useWorkspaceMRsMock(workspaceId),
}));
vi.mock("@/hooks/domains/github/use-task-pr", () => ({
  useWorkspacePRs: (workspaceId: string | null) => useWorkspacePRsMock(workspaceId),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/lib/api", () => ({
  archiveTask: vi.fn(),
  deleteTask: vi.fn(),
  listTasksByWorkspace: vi.fn(),
  unarchiveTask: vi.fn(),
  updateUserSettings: vi.fn(),
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));
vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: () => ({
    activeWorkspaceId: "ws1",
    activeWorkflowId: null,
    repositories: [],
    selectedRepositoryId: null,
  }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: (value: unknown) => value,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: "desktop",
    isMobile: false,
    isTablet: false,
    isDesktop: true,
    isCompactDesktop: false,
    isFullDesktop: true,
    isFinePointer: true,
    usesDesktopWorkbench: true,
  }),
}));
vi.mock("@/hooks/use-task-listing-view", () => ({
  useTaskListingView: () => ({ effectiveView: "list", setView: vi.fn() }),
}));
vi.mock("@/hooks/use-foreground-refresh", () => ({
  useForegroundRefresh: () => undefined,
}));
vi.mock("@/hooks/use-workflow-snapshot", () => ({
  useWorkflowSnapshot: () => undefined,
}));
vi.mock("@/components/kanban/kanban-header", () => ({
  KanbanHeader: () => null,
}));
vi.mock("@/components/kanban/mobile-fab", () => ({
  MobileFab: () => null,
}));
vi.mock("@/components/kanban/mobile-search-bar", () => ({
  MobileSearchBar: () => null,
}));
vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: () => null,
}));
vi.mock("./tasks-list-view", () => ({
  TasksListView: () => null,
}));

import { TasksPageClient } from "./tasks-page-client";

afterEach(() => {
  cleanup();
  useWorkspaceMRsMock.mockReset();
  useWorkspacePRsMock.mockReset();
});

function renderPage(tasksListShowDetails: boolean) {
  return render(
    <StateProvider
      initialState={{
        workspaces: { items: [], activeId: "ws1" },
        userSettings: { ...defaultSettingsState.userSettings, tasksListShowDetails, loaded: true },
      }}
    >
      <TasksPageClient
        workspaces={[]}
        initialWorkflows={[]}
        initialRepositories={[]}
        initialTasks={[]}
        initialTotal={0}
        initialDataLoaded
        initialSort="updated_desc"
        initialGroup="none"
      />
    </StateProvider>,
  );
}

describe("TasksPageClient change-request hydration (AC12)", () => {
  it("hydrates PRs and MRs with the workspace id when task details are off", () => {
    renderPage(false);

    expect(useWorkspaceMRsMock).toHaveBeenCalledWith("ws1");
    expect(useWorkspacePRsMock).toHaveBeenCalledWith("ws1");
  });

  it("calls useWorkspaceMRs with the workspace id when task details are on", () => {
    renderPage(true);

    expect(useWorkspaceMRsMock).toHaveBeenCalledWith("ws1");
    expect(useWorkspacePRsMock).toHaveBeenCalledWith("ws1");
  });
});
