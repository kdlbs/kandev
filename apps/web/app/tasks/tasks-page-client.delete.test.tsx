import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import type { DeleteTaskParams } from "@/lib/api/domains/kanban-api";

const { deleteTask, toast, responsive } = vi.hoisted(() => ({
  deleteTask: vi.fn(),
  toast: vi.fn(),
  responsive: { isMobile: false },
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/lib/api", () => ({
  archiveTask: vi.fn(),
  deleteTask,
  unarchiveTask: vi.fn(),
  listTasksByWorkspace: vi.fn().mockResolvedValue({ tasks: [], total: 0 }),
  updateUserSettings: vi.fn().mockResolvedValue({}),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast }) }));
vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: () => ({
    activeWorkspaceId: "ws1",
    activeWorkflowId: null,
    repositories: [],
    selectedRepositoryId: null,
  }),
}));
vi.mock("@/hooks/use-debounce", () => ({ useDebounce: (value: unknown) => value }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));
vi.mock("@/hooks/use-task-listing-view", () => ({
  useTaskListingView: () => ({ effectiveView: "list", setView: vi.fn() }),
}));
vi.mock("@/hooks/use-foreground-refresh", () => ({ useForegroundRefresh: () => undefined }));
vi.mock("@/hooks/use-workflow-snapshot", () => ({ useWorkflowSnapshot: () => undefined }));
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({ useWorkspaceMRs: () => undefined }));
vi.mock("@/hooks/domains/github/use-task-pr", () => ({ useWorkspacePRs: () => undefined }));
vi.mock("./tasks-page-content", () => ({
  TasksPageContent: ({
    onDelete,
  }: {
    onDelete: (taskId: string, opts?: DeleteTaskParams) => Promise<void>;
  }) => (
    <button
      onClick={() =>
        void onDelete("damaged-task", {
          discardWorktreeChanges: true,
          allowUnverifiedWorkspaceChanges: true,
        })
      }
    >
      Delete task
    </button>
  ),
}));
import { TasksPageClient } from "./tasks-page-client";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe.each([false, true])("Tasks page deletion (mobile=%s)", (isMobile) => {
  // @covers AC-TASKS-DELETE-RECOVERY-002.8
  it("reports pending workspace cleanup after successful task deletion", async () => {
    responsive.isMobile = isMobile;
    deleteTask.mockResolvedValue({
      workspace_cleanup: "delete",
      workspace_cleanup_pending: true,
    });
    render(
      <StateProvider
        initialState={{
          workspaces: { items: [], activeId: "ws1" },
          userSettings: { ...defaultSettingsState.userSettings, loaded: true },
        }}
      >
        <TasksPageClient
          workspaces={[]}
          initialRepositories={[]}
          initialTasks={[]}
          initialTotal={0}
          initialDataLoaded
          initialSort="updated_desc"
          initialGroup="none"
        />
      </StateProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete task" }));

    await waitFor(() =>
      expect(toast).toHaveBeenCalledWith({
        title: "Task deleted. Cleanup pending.",
        description:
          "Files that cannot be safely removed remain protected. Kandev will retry cleanup.",
      }),
    );
    expect(deleteTask).toHaveBeenCalledWith("damaged-task", {
      discardWorktreeChanges: true,
      allowUnverifiedWorkspaceChanges: true,
    });
  });
});
