import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskTopBarTitle } from "./task-top-bar-title";

const mockRename = vi.hoisted(() => vi.fn(() => Promise.resolve()));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({ renameTaskById: mockRename }),
}));

afterEach(() => {
  cleanup();
  mockRename.mockClear();
});

function getTitle() {
  return screen.getByText("My task", { selector: '[aria-current="page"]' });
}

function queryInput() {
  return screen.queryByTestId("task-title-rename-input");
}

function startEditing() {
  fireEvent.doubleClick(getTitle());
  return screen.getByTestId("task-title-rename-input") as HTMLInputElement;
}

describe("TaskTopBarTitle", () => {
  it("renders the title as the breadcrumb page when idle", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    expect(getTitle()).toBeTruthy();
    expect(queryInput()).toBeNull();
    // Not aria-disabled, so pointer-actionability checks treat it as interactive.
    expect(getTitle().getAttribute("aria-disabled")).toBe("false");
  });

  it("keeps the breadcrumb aria-disabled when the task is archived", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" isArchived />);

    expect(getTitle().getAttribute("aria-disabled")).toBe("true");
  });

  it("swaps to an input pre-filled with the current title on double-click", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    const input = startEditing();

    expect(input.value).toBe("My task");
  });

  it("renames on Enter with a changed value, trimming whitespace", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    const input = startEditing();
    fireEvent.change(input, { target: { value: "  New title  " } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(mockRename).toHaveBeenCalledWith("task-1", "New title");
    expect(queryInput()).toBeNull();
  });

  it("does not rename on Enter when the value is unchanged", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    const input = startEditing();
    fireEvent.keyDown(input, { key: "Enter" });

    expect(mockRename).not.toHaveBeenCalled();
    expect(queryInput()).toBeNull();
  });

  it("does not rename on Enter when the value is whitespace-only", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    const input = startEditing();
    fireEvent.change(input, { target: { value: "   " } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(mockRename).not.toHaveBeenCalled();
    expect(queryInput()).toBeNull();
  });

  it("ignores Enter fired during IME composition", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    const input = startEditing();
    fireEvent.change(input, { target: { value: "New title" } });
    fireEvent.keyDown(input, { key: "Enter", isComposing: true });

    expect(mockRename).not.toHaveBeenCalled();
    expect(queryInput()).not.toBeNull();
  });

  it("ignores the IME-accepting Enter reported as keyCode 229", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    const input = startEditing();
    fireEvent.change(input, { target: { value: "New title" } });
    fireEvent.keyDown(input, { key: "Enter", keyCode: 229 });

    expect(mockRename).not.toHaveBeenCalled();
    expect(queryInput()).not.toBeNull();
  });

  it("does not rename on Enter when the task was archived mid-edit", () => {
    const { rerender } = render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    const input = startEditing();
    fireEvent.change(input, { target: { value: "New title" } });
    rerender(<TaskTopBarTitle taskId="task-1" taskTitle="My task" isArchived />);
    fireEvent.keyDown(input, { key: "Enter" });

    expect(mockRename).not.toHaveBeenCalled();
    expect(queryInput()).toBeNull();
  });

  it("cancels on Escape without renaming", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    const input = startEditing();
    fireEvent.change(input, { target: { value: "New title" } });
    fireEvent.keyDown(input, { key: "Escape" });

    expect(mockRename).not.toHaveBeenCalled();
    expect(queryInput()).toBeNull();
    expect(getTitle()).toBeTruthy();
  });

  it("cancels on blur without renaming", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    const input = startEditing();
    fireEvent.change(input, { target: { value: "New title" } });
    fireEvent.blur(input);

    expect(mockRename).not.toHaveBeenCalled();
    expect(queryInput()).toBeNull();
  });

  it("does not enter edit mode when the task is archived", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" isArchived />);

    fireEvent.doubleClick(getTitle());

    expect(queryInput()).toBeNull();
  });

  it("does not enter edit mode without a task id", () => {
    render(<TaskTopBarTitle taskTitle="My task" />);

    fireEvent.doubleClick(getTitle());

    expect(queryInput()).toBeNull();
  });
});
