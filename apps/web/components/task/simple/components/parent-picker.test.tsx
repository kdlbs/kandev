import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useEffect, type ReactNode } from "react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import type { Task } from "@/app/office/tasks/[id]/types";
import { ParentPicker } from "./parent-picker";

const detachTaskMock = vi.hoisted(() => vi.fn().mockResolvedValue({ id: "child" }));
const fetchTaskMock = vi.hoisted(() =>
  vi.fn().mockResolvedValue({
    id: "child",
    metadata: { workspace: { mode: "inherit_parent", group_id: "group-1" } },
  }),
);
const updateTaskMock = vi.hoisted(() => vi.fn().mockResolvedValue({ ok: true }));

vi.mock("@/lib/api/domains/kanban-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/kanban-api")>(
    "@/lib/api/domains/kanban-api",
  );
  return { ...actual, detachTask: detachTaskMock, fetchTask: fetchTaskMock };
});

vi.mock("@/lib/api/domains/office-extended-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-extended-api")>(
    "@/lib/api/domains/office-extended-api",
  );
  return {
    ...actual,
    updateTask: updateTaskMock,
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const task: Task = {
  id: "child",
  workspaceId: "ws-1",
  identifier: "TASK-2",
  title: "Child task",
  status: "todo",
  priority: "medium",
  parentId: "parent-1",
  parentTitle: "Parent one",
  parentIdentifier: "TASK-1",
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

const candidates = [
  {
    id: "parent-1",
    workspaceId: "ws-1",
    identifier: "TASK-1",
    title: "Parent one",
  },
  {
    id: "parent-2",
    workspaceId: "ws-1",
    identifier: "TASK-3",
    title: "Parent two",
  },
] as OfficeTask[];

function SeedTasks() {
  const setTasks = useAppStore((state) => state.setTasks);
  useEffect(() => setTasks(candidates), [setTasks]);
  return null;
}

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <StateProvider>
      <SeedTasks />
      <TaskOptimisticContextProvider value={{ task, applyPatch: vi.fn(), restore: vi.fn() }}>
        {children}
      </TaskOptimisticContextProvider>
    </StateProvider>
  );
}

describe("ParentPicker", () => {
  it("uses canonical detach after confirming No parent", async () => {
    render(
      <Wrapper>
        <ParentPicker task={task} />
      </Wrapper>,
    );

    await waitFor(() => expect(fetchTaskMock).toHaveBeenCalledWith(task.id));

    fireEvent.click(screen.getByTestId("parent-picker-trigger"));
    fireEvent.click(await screen.findByText("No parent"));
    const dialog = await screen.findByRole("alertdialog", { name: "Detach task from parent?" });
    expect(dialog.textContent).toContain("shares its parent's workspace");
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Detach" }));
    });

    expect(detachTaskMock).toHaveBeenCalledWith(task.id);
    expect(updateTaskMock).not.toHaveBeenCalled();
  });

  it("keeps non-empty reparenting on the Office update endpoint", async () => {
    render(
      <Wrapper>
        <ParentPicker task={task} />
      </Wrapper>,
    );

    fireEvent.click(screen.getByTestId("parent-picker-trigger"));
    await act(async () => {
      fireEvent.click(await screen.findByText("Parent two"));
    });

    expect(updateTaskMock).toHaveBeenCalledWith(task.id, { parent_id: "parent-2" });
    expect(detachTaskMock).not.toHaveBeenCalled();
  });
});
