import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";

const mockGetSubtaskCount = vi.fn();

vi.mock("@/lib/api", () => ({
  getSubtaskCount: (...args: unknown[]) => mockGetSubtaskCount(...args),
}));

import { TaskDeleteConfirmDialog } from "./task-delete-confirm-dialog";

beforeEach(() => {
  mockGetSubtaskCount.mockReset();
});

afterEach(cleanup);

describe("TaskDeleteConfirmDialog", () => {
  it("hides the cascade checkbox when the task has no subtasks", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    const onConfirm = vi.fn();
    render(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        onConfirm={onConfirm}
        confirmTestId="confirm"
      />,
    );
    await waitFor(() => expect(mockGetSubtaskCount).toHaveBeenCalledWith("task-1"));
    expect(screen.queryByTestId("delete-cascade-checkbox")).toBeNull();

    fireEvent.click(screen.getByTestId("confirm"));
    expect(onConfirm).toHaveBeenCalledWith({ cascade: false });
  });

  it("shows the cascade checkbox when the task has subtasks; defaults to unchecked", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 3 });
    const onConfirm = vi.fn();
    render(
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
    render(
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
    render(
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
