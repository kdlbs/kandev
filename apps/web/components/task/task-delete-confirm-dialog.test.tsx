import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

const mockGetSubtaskCount = vi.fn();

vi.mock("@/lib/api/domains/kanban-api", () => ({
  getSubtaskCount: (...args: unknown[]) => mockGetSubtaskCount(...args),
}));

import { TaskDeleteConfirmDialog } from "./task-delete-confirm-dialog";

function renderWithQuery(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>);
}

beforeEach(() => {
  mockGetSubtaskCount.mockReset();
});

afterEach(cleanup);

describe("TaskDeleteConfirmDialog", () => {
  it("hides the cascade checkbox when the task has no subtasks", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    const onConfirm = vi.fn();
    renderWithQuery(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        onConfirm={onConfirm}
        confirmTestId="confirm"
      />,
    );
    await waitFor(() =>
      expect(mockGetSubtaskCount).toHaveBeenCalledWith("task-1", {
        init: { signal: expect.any(AbortSignal) },
      }),
    );
    expect(screen.queryByTestId("delete-cascade-checkbox")).toBeNull();

    fireEvent.click(screen.getByTestId("confirm"));
    expect(onConfirm).toHaveBeenCalledWith({ cascade: false });
  });

  it("shows the cascade checkbox when the task has subtasks; defaults to unchecked", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 3 });
    const onConfirm = vi.fn();
    renderWithQuery(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        onConfirm={onConfirm}
        confirmTestId="confirm"
      />,
    );
    await screen.findByTestId("delete-cascade-checkbox");
    expect(screen.getByText(/Also delete 3 subtasks/i)).toBeTruthy();

    fireEvent.click(screen.getByTestId("confirm"));
    expect(onConfirm).toHaveBeenCalledWith({ cascade: false });
  });

  it("propagates cascade=true when the user ticks the checkbox", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 2 });
    const onConfirm = vi.fn();
    renderWithQuery(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        onConfirm={onConfirm}
        confirmTestId="confirm"
      />,
    );
    const checkbox = await screen.findByTestId("delete-cascade-checkbox");
    fireEvent.click(checkbox);
    fireEvent.click(screen.getByTestId("confirm"));
    expect(onConfirm).toHaveBeenCalledWith({ cascade: true });
  });

  it("sums subtask counts across taskIds for bulk delete", async () => {
    mockGetSubtaskCount.mockImplementation((id: string) =>
      Promise.resolve({ count: id === "a" ? 2 : 5 }),
    );
    renderWithQuery(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        isBulkOperation
        count={2}
        taskIds={["a", "b"]}
        onConfirm={() => {}}
      />,
    );
    await screen.findByText(/Also delete 7 subtasks/i);
  });
});

describe("TaskDeleteConfirmDialog executor cleanup copy", () => {
  it("local reassures repo is untouched", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderWithQuery(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="local"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/directly in your repo/i)).toBeTruthy();
    expect(screen.getByText(/not touched/i)).toBeTruthy();
  });

  it("worktree describes worktree+branch removal", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderWithQuery(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="worktree"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/worktree and its branch will be deleted/i)).toBeTruthy();
  });

  it("groups bulk delete copy by executor type", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderWithQuery(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        isBulkOperation
        count={3}
        taskIds={["a", "b", "c"]}
        executorTypes={["worktree", "worktree", "local"]}
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/2 worktrees/i)).toBeTruthy();
    expect(screen.getByText(/1 local task/i)).toBeTruthy();
  });

  it("falls back to a generic message when no executorType is provided", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderWithQuery(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/Any running agent sessions will be stopped/i)).toBeTruthy();
  });
});
