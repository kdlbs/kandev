import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("./task-note-panel", () => ({
  TaskNotePanel: ({ taskId }: { taskId: string }) => (
    <div data-testid="task-note-panel">{taskId}</div>
  ),
}));

import { TaskNoteEditDialog } from "./task-note-edit-dialog";

describe("TaskNoteEditDialog", () => {
  it("renders the note panel for the selected task", () => {
    render(<TaskNoteEditDialog open onOpenChange={vi.fn()} taskId="task-1" taskTitle="My task" />);

    expect(screen.getByText(/Edit notes — My task/i)).toBeTruthy();
    expect(screen.getByTestId("task-note-panel").textContent).toBe("task-1");
  });
});
