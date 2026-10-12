import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CoordinatorDeleteConfirmDialog } from "./coordinator-delete-confirm-dialog";

describe("CoordinatorDeleteConfirmDialog", () => {
  afterEach(cleanup);

  it("says the action cannot be undone and what is deleted vs. kept (AC-004.5)", () => {
    render(
      <CoordinatorDeleteConfirmDialog
        open
        coordinatorName="Planner"
        isDeleting={false}
        onOpenChange={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    const dialog = screen.getByRole("alertdialog");
    expect(dialog.textContent).toContain("Planner");
    expect(dialog.textContent).toContain("can't be undone");
    expect(dialog.textContent).toContain("Pending proposals and the conversation are deleted");
    expect(dialog.textContent).toContain("tasks it already created stay");
  });

  it("confirms only on the destructive action, not Cancel", () => {
    const onConfirm = vi.fn();
    render(
      <CoordinatorDeleteConfirmDialog
        open
        coordinatorName="Planner"
        isDeleting={false}
        onOpenChange={vi.fn()}
        onConfirm={onConfirm}
      />,
    );

    fireEvent.click(screen.getByTestId("coordinator-delete-confirm"));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("disables both actions while deleting is in flight", () => {
    render(
      <CoordinatorDeleteConfirmDialog
        open
        coordinatorName="Planner"
        isDeleting
        onOpenChange={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    expect(screen.getByTestId("coordinator-delete-confirm").hasAttribute("disabled")).toBe(true);
    expect(screen.getByRole("button", { name: /cancel/i }).hasAttribute("disabled")).toBe(true);
  });

  it("renders nothing when closed", () => {
    render(
      <CoordinatorDeleteConfirmDialog
        open={false}
        coordinatorName="Planner"
        isDeleting={false}
        onOpenChange={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });
});
