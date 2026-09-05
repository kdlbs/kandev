import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { TaskAssigneeControl } from "./task-assignee-control";
import { resetDirectoryCacheForTests } from "@/hooks/domains/users/use-assignable-people";

const CONTROL = "task-assignee-control";

vi.mock("@/lib/api/domains/kanban-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/kanban-api")>(
    "@/lib/api/domains/kanban-api",
  );
  return { ...actual, updateTask: vi.fn().mockResolvedValue({}) };
});

vi.mock("@/lib/api/domains/team-access-api", () => ({
  listWorkspaceMembers: vi.fn().mockResolvedValue({
    members: [{ user_id: "user-1", display_name: "Ada Lovelace", role: "owner" }],
    total: 1,
  }),
  listDirectoryUsers: vi.fn().mockResolvedValue({
    users: [
      { id: "user-1", display_name: "Ada Lovelace" },
      { id: "user-2", display_name: "Grace Hopper" },
    ],
    total: 2,
  }),
}));

import { updateTask } from "@/lib/api/domains/kanban-api";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  resetDirectoryCacheForTests();
});

function stateWith(assigneeUserId?: string, authenticated = true) {
  return {
    auth: {
      mode: authenticated ? ("enabled" as const) : ("disabled" as const),
      authenticated: true,
      user: authenticated
        ? {
            id: "user-2",
            email: "grace@example.com",
            display_name: "Grace Hopper",
            role: "member",
            status: "active",
          }
        : null,
      ssoProviders: [],
    },
    kanban: {
      workflowId: "wf-1",
      steps: [],
      tasks: [
        {
          id: "t-1",
          title: "First task",
          workflowId: "wf-1",
          workflowStepId: "step-1",
          position: 0,
          assigneeUserId,
        },
      ],
    },
  };
}

function renderControl(assigneeUserId?: string, authenticated = true) {
  return render(
    <StateProvider initialState={stateWith(assigneeUserId, authenticated)}>
      <TaskAssigneeControl taskId="t-1" workspaceId="ws-1" />
    </StateProvider>,
  );
}

describe("TaskAssigneeControl", () => {
  it("shows the assignee's name from the store, not a raw user id", async () => {
    renderControl("user-1");
    await waitFor(() => expect(screen.getByTestId(CONTROL).textContent).toContain("Ada Lovelace"));
  });

  it("assigns to the signed-in user from the dropdown", async () => {
    renderControl();
    fireEvent.click(screen.getByTestId(CONTROL));
    fireEvent.click(await screen.findByText("Assign to me"));
    await waitFor(() =>
      expect(updateTask).toHaveBeenCalledWith("t-1", { assignee_user_id: "user-2" }),
    );
  });

  it("omits Assign to me when the task is already the caller's", async () => {
    renderControl("user-2");
    fireEvent.click(screen.getByTestId(CONTROL));
    await screen.findByText("Unassigned");
    expect(screen.queryByText("Assign to me")).toBeNull();
  });

  it("sends an empty string to unassign", async () => {
    renderControl("user-1");
    fireEvent.click(screen.getByTestId(CONTROL));
    fireEvent.click(await screen.findByText("Unassigned"));
    await waitFor(() => expect(updateTask).toHaveBeenCalledWith("t-1", { assignee_user_id: "" }));
  });

  it("renders nothing when authentication is disabled", () => {
    renderControl(undefined, false);
    expect(screen.queryByTestId(CONTROL)).toBeNull();
  });
});
