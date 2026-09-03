import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const touch = vi.hoisted(() => ({ enabled: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touch.enabled,
}));

import { PlanSelectionPopover } from "./plan-selection-popover";

const SAVED_FEEDBACK = "Keep this feedback";

afterEach(() => {
  touch.enabled = false;
  cleanup();
});

describe("PlanSelectionPopover", () => {
  it("keeps entered feedback open when the submitter rejects the save", () => {
    const onClose = vi.fn();
    render(
      <PlanSelectionPopover
        selectedText="settled answer"
        position={{ x: 100, y: 100 }}
        onAdd={() => false}
        onClose={onClose}
      />,
    );

    const input = screen.getByPlaceholderText("Add your comment or instruction...");
    fireEvent.change(input, { target: { value: SAVED_FEEDBACK } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    expect(onClose).not.toHaveBeenCalled();
    expect((input as HTMLTextAreaElement).value).toBe(SAVED_FEEDBACK);
  });

  it("waits for persistence and keeps entered feedback when the async save fails", async () => {
    const onClose = vi.fn();
    const onAdd = vi.fn().mockResolvedValue(false);
    render(
      <PlanSelectionPopover
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        onAdd={onAdd}
        onClose={onClose}
        errorMessage="Could not save this comment. Try again."
      />,
    );

    const input = screen.getByPlaceholderText("Add your comment or instruction...");
    fireEvent.change(input, { target: { value: SAVED_FEEDBACK } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    await waitFor(() => expect(onAdd).toHaveBeenCalled());
    expect(onClose).not.toHaveBeenCalled();
    expect((input as HTMLTextAreaElement).value).toBe(SAVED_FEEDBACK);
    expect(screen.getByRole("alert").textContent).toContain("Could not save this comment");
  });

  it("disables Run and exposes the primary-session reason", () => {
    render(
      <PlanSelectionPopover
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        onAdd={() => true}
        onAddAndRun={() => true}
        runDisabledReason="This task has no primary session available for Run."
        onClose={() => undefined}
      />,
    );

    expect(screen.getByRole("button", { name: "Run" }).hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("This task has no primary session available for Run.")).toBeTruthy();
  });

  it("keeps an edited comment open when async deletion is rejected", async () => {
    const onClose = vi.fn();
    const onDelete = vi.fn().mockResolvedValue(false);
    render(
      <PlanSelectionPopover
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        editingComment="Keep this"
        onAdd={() => true}
        onDelete={onDelete}
        onClose={onClose}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete comment" }));

    await waitFor(() => expect(onDelete).toHaveBeenCalled());
    expect(onClose).not.toHaveBeenCalled();
  });

  it("uses a scroll-contained drawer with touch-sized actions on coarse pointers", () => {
    touch.enabled = true;
    render(
      <PlanSelectionPopover
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        onAdd={() => true}
        onAddAndRun={() => true}
        onClose={() => undefined}
      />,
    );

    expect(screen.getByTestId("plan-comment-drawer").className).toContain("max-h-[82dvh]");
    expect(screen.getByRole("button", { name: "Add" }).className).toContain("min-h-11");
    expect(screen.getByRole("button", { name: "Run" }).className).toContain("min-h-11");
  });
});
