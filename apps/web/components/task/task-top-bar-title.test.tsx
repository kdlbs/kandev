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

describe("TaskTopBarTitle — idle state", () => {
  it("renders the title as the breadcrumb page when idle", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    expect(getTitle()).toBeTruthy();
    expect(queryInput()).toBeNull();
    // Not aria-disabled, so pointer-actionability checks treat it as interactive.
    expect(getTitle().getAttribute("aria-disabled")).toBe("false");
    // Keyboard-operable: reachable via Tab.
    expect(getTitle().getAttribute("tabindex")).toBe("0");
  });

  it("keeps the breadcrumb aria-disabled and unfocusable when the task is archived", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" isArchived />);

    expect(getTitle().getAttribute("aria-disabled")).toBe("true");
    expect(getTitle().getAttribute("tabindex")).toBeNull();
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

describe("TaskTopBarTitle — entering edit mode", () => {
  it("swaps to an input pre-filled with the current title on double-click", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    const input = startEditing();

    expect(input.value).toBe("My task");
  });

  it("enters edit mode on Enter when the title is focused", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    fireEvent.keyDown(getTitle(), { key: "Enter" });

    const input = screen.getByTestId("task-title-rename-input") as HTMLInputElement;
    expect(input.value).toBe("My task");
  });

  it("does not enter edit mode on title Enter when the task is archived", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" isArchived />);

    fireEvent.keyDown(getTitle(), { key: "Enter" });

    expect(queryInput()).toBeNull();
  });
});

describe("TaskTopBarTitle — committing a rename", () => {
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
});

describe("TaskTopBarTitle — cancelling a rename", () => {
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

  it("returns focus to the title after a keyboard exit", () => {
    render(<TaskTopBarTitle taskId="task-1" taskTitle="My task" />);

    const input = startEditing();
    fireEvent.keyDown(input, { key: "Escape" });

    expect(document.activeElement).toBe(getTitle());
  });
});
