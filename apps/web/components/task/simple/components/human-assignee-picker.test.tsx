import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import { HumanAssigneePicker } from "./human-assignee-picker";
import type { Task } from "@/app/office/tasks/[id]/types";

vi.mock("@/lib/api/domains/office-extended-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-extended-api")>(
    "@/lib/api/domains/office-extended-api",
  );
  return { ...actual, updateTask: vi.fn().mockResolvedValue({ ok: true }) };
});

vi.mock("@/lib/api/domains/team-access-api", () => ({
  // Ada holds a member row; Bruno reaches the workspace through org
  // visibility and appears only in the directory. Both must resolve to a
  // name, which is what the member list alone could not do.
  listWorkspaceMembers: vi.fn().mockResolvedValue({
    members: [{ user_id: "user-1", display_name: "Ada Lovelace", role: "owner" }],
    total: 1,
  }),
  listDirectoryUsers: vi.fn().mockResolvedValue({
    users: [
      { id: "user-2", display_name: "Grace Hopper" },
      { id: "user-3", display_name: "Bruno Costa" },
    ],
    total: 2,
  }),
}));

import { updateTask } from "@/lib/api/domains/office-extended-api";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const baseTask: Task = {
  id: "t-1",
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: "First task",
  status: "todo",
  priority: "medium",
  labels: [],
  blockedBy: [],
  blocking: [],
  children: [],
  reviewers: [],
  approvers: [],
  decisions: [],
  createdBy: "user",
  createdAt: "2026-05-01T00:00:00Z",
  updatedAt: "2026-05-01T00:00:00Z",
};

const initialState = {
  auth: {
    mode: "enabled" as const,
    authenticated: true,
    user: {
      id: "user-2",
      email: "grace@example.com",
      display_name: "Grace Hopper",
      role: "member",
      status: "active",
    },
    ssoProviders: [],
  },
  workspaces: { activeId: "ws-1", items: [] },
};

function Wrapper({ children, task }: { children: ReactNode; task: Task }) {
  const ctx = { task, applyPatch: vi.fn(), restore: vi.fn() };
  return (
    <StateProvider initialState={initialState}>
      <TaskOptimisticContextProvider value={ctx}>{children}</TaskOptimisticContextProvider>
    </StateProvider>
  );
}

describe("HumanAssigneePicker", () => {
  it("assigns the task to the signed-in user when Assign to me is clicked", async () => {
    render(
      <Wrapper task={baseTask}>
        <HumanAssigneePicker task={baseTask} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId("assign-to-me"));
    await waitFor(() =>
      expect(updateTask).toHaveBeenCalledWith("t-1", { assignee_user_id: "user-2" }),
    );
  });

  it("hides Assign to me when the task is already the caller's", () => {
    const mine: Task = { ...baseTask, assigneeUserId: "user-2" };
    render(
      <Wrapper task={mine}>
        <HumanAssigneePicker task={mine} />
      </Wrapper>,
    );
    expect(screen.queryByTestId("assign-to-me")).toBeNull();
  });

  it("shows the assignee's display name once members load", async () => {
    const assigned: Task = { ...baseTask, assigneeUserId: "user-1" };
    render(
      <Wrapper task={assigned}>
        <HumanAssigneePicker task={assigned} />
      </Wrapper>,
    );
    await waitFor(() =>
      expect(screen.getByTestId("human-assignee-picker-trigger").textContent).toContain(
        "Ada Lovelace",
      ),
    );
  });

  it("resolves a name for someone who reaches the workspace without a member row", async () => {
    const assigned: Task = { ...baseTask, assigneeUserId: "user-3" };
    render(
      <Wrapper task={assigned}>
        <HumanAssigneePicker task={assigned} />
      </Wrapper>,
    );
    await waitFor(() =>
      expect(screen.getByTestId("human-assignee-picker-trigger").textContent).toContain(
        "Bruno Costa",
      ),
    );
  });

  it("sends an empty string to unassign, which is distinct from omitting the field", async () => {
    const assigned: Task = { ...baseTask, assigneeUserId: "user-1" };
    render(
      <Wrapper task={assigned}>
        <HumanAssigneePicker task={assigned} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId("human-assignee-picker-trigger"));
    fireEvent.click(await screen.findByText("Unassigned"));
    await waitFor(() => expect(updateTask).toHaveBeenCalledWith("t-1", { assignee_user_id: "" }));
  });
});
