import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskNavigationIdentity } from "@/lib/state/task-navigation-reads";
import { taskId, workspaceId, workflowId } from "@/lib/types/ids";
import { TaskDetailRoute } from "./task-detail-route";

const mocks = vi.hoisted(() => ({
  readIdentity: vi.fn(),
  enrich: vi.fn(),
}));
const FIRST_TASK = "previous-task";
const NEXT_TASK = "destination-task";
const FIRST_WORKSPACE = "previous-workspace";
const NEXT_WORKSPACE = "destination-workspace";
const SHELL_TEST_ID = "task-shell";

vi.mock("@/lib/state/task-navigation-reads", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/state/task-navigation-reads")>()),
  readTaskNavigationIdentity: mocks.readIdentity,
}));
vi.mock("@/lib/ssr/session-page-state", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/ssr/session-page-state")>()),
  fetchTaskNavigationEnrichment: mocks.enrich,
}));
vi.mock("@/app/tasks/[id]/kanban-task-shell", () => ({
  KanbanTaskShell: ({ taskId }: { taskId: string }) => (
    <div data-testid={SHELL_TEST_ID}>{taskId}</div>
  ),
}));

function identity(id: string, owner: string): TaskNavigationIdentity {
  return {
    task: {
      id: taskId(id),
      workspace_id: workspaceId(owner),
      workflow_id: workflowId("workflow"),
      workflow_step_id: "step",
      title: id,
      description: "",
      state: "TODO",
      priority: "medium",
      position: 0,
      repositories: [],
      created_at: "2026-09-30T00:00:00Z",
      updated_at: "2026-09-30T00:00:00Z",
    },
    allSessionsResponse: { sessions: [], total: 0 },
  };
}

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

it("does not reload the old route when workspace selection changes before navigation", async () => {
  mocks.readIdentity.mockResolvedValueOnce(identity(FIRST_TASK, FIRST_WORKSPACE));
  mocks.enrich.mockImplementation((data: TaskNavigationIdentity) =>
    Promise.resolve({
      task: data.task,
      sessionId: null,
      initialTerminals: [],
      initialState: { workspaces: { items: [], activeId: data.task.workspace_id } },
    }),
  );
  const initialState = {
    workspaces: { items: [], activeId: FIRST_WORKSPACE },
    kanban: {
      workflowId: "workflow",
      steps: [],
      tasks: [
        {
          id: FIRST_TASK,
          title: FIRST_TASK,
          workspaceId: FIRST_WORKSPACE,
          workflowId: "workflow",
          workflowStepId: "step",
          position: 0,
        },
      ],
    },
  } as Partial<AppState>;
  const { rerender } = render(<TaskDetailRoute taskId={FIRST_TASK} />, {
    wrapper: ({ children }) => (
      <StateProvider initialState={initialState}>{children}</StateProvider>
    ),
  });
  await waitFor(() => expect(mocks.enrich).toHaveBeenCalledTimes(1));
  const store = mocks.readIdentity.mock.calls[0][0] as StoreApi<AppState>;
  act(() => store.getState().setActiveWorkspace(NEXT_WORKSPACE));
  rerender(<TaskDetailRoute taskId={FIRST_TASK} />);
  expect(mocks.readIdentity).toHaveBeenCalledTimes(1);
  expect(store.getState().workspaces.activeId).toBe(NEXT_WORKSPACE);

  mocks.readIdentity.mockResolvedValueOnce(identity(NEXT_TASK, NEXT_WORKSPACE));
  rerender(<TaskDetailRoute taskId={NEXT_TASK} />);
  await waitFor(() => expect(screen.getByTestId(SHELL_TEST_ID).textContent).toBe(NEXT_TASK));
  expect(mocks.readIdentity).toHaveBeenCalledTimes(2);
  expect(store.getState().workspaces.activeId).toBe(NEXT_WORKSPACE);
});
