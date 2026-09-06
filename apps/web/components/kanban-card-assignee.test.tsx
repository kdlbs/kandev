import { render, screen, cleanup, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mockAppState = {
  workspaces: { activeId: "ws-1" },
  kanban: { tasks: [] as Array<{ id: string; title: string }> },
  kanbanMulti: { snapshots: {} as Record<string, { tasks: Array<{ id: string }> }> },
  taskPRs: { byTaskId: {} as Record<string, unknown> },
  taskMRs: { byWorkspaceId: {} as Record<string, Record<string, unknown>> },
};

// The card body reaches the store two ways: useAppStore for the slices above,
// and useAppStoreApi for the imperative read the action menu does. A mock that
// defines only the first throws on render, so the assignee assertions below
// would fail for a reason that has nothing to do with the assignee.
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockAppState) => unknown) => selector(mockAppState),
  useAppStoreApi: () => ({
    getState: () => mockAppState,
    setState: vi.fn(),
    subscribe: () => () => undefined,
  }),
}));

vi.mock("@/lib/api/domains/team-access-api", () => ({
  listDirectoryUsers: vi
    .fn()
    .mockResolvedValue({ users: [{ id: "user-1", display_name: "Ada Lovelace" }], total: 1 }),
  listWorkspaceMembers: vi.fn().mockResolvedValue({ members: [], total: 0 }),
}));

import { listDirectoryUsers } from "@/lib/api/domains/team-access-api";
import { resetDirectoryCacheForTests } from "@/hooks/domains/users/use-assignable-people";
import { KanbanCardBody } from "./kanban-card-content";
import type { Task } from "./kanban-card";

const TASK: Task = { id: "task-1", title: "Fix the bug", workflowStepId: "step-1" };

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  resetDirectoryCacheForTests();
});

describe("KanbanCardBody — human assignee", () => {
  it("shows the assignee's name so a board answers who is on what", async () => {
    render(<KanbanCardBody task={{ ...TASK, assigneeUserId: "user-1" }} repositoryChips={[]} />);

    await waitFor(() =>
      expect(screen.getByTestId("kanban-card-assignee").textContent).toContain("Ada Lovelace"),
    );
  });

  it("renders no assignee affordance on an unassigned task", () => {
    render(<KanbanCardBody task={TASK} repositoryChips={[]} />);
    expect(screen.queryByTestId("kanban-card-assignee")).toBeNull();
  });

  it("fetches the directory once for a board full of cards", async () => {
    render(
      <>
        <KanbanCardBody task={{ ...TASK, assigneeUserId: "user-1" }} repositoryChips={[]} />
        <KanbanCardBody
          task={{ ...TASK, id: "task-2", assigneeUserId: "user-1" }}
          repositoryChips={[]}
        />
        <KanbanCardBody
          task={{ ...TASK, id: "task-3", assigneeUserId: "user-1" }}
          repositoryChips={[]}
        />
      </>,
    );

    await waitFor(() => expect(screen.getAllByTestId("kanban-card-assignee")).toHaveLength(3));
    expect(listDirectoryUsers).toHaveBeenCalledTimes(1);
  });
});
