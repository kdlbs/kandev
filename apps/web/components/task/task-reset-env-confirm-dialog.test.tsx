import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TaskResetEnvConfirmDialog } from "./task-reset-env-confirm-dialog";

afterEach(cleanup);

describe("TaskResetEnvConfirmDialog", () => {
  it("renders structured warning copy through one left-aligned description", () => {
    render(
      <TaskResetEnvConfirmDialog open onOpenChange={vi.fn()} hasWorktreePath onConfirm={vi.fn()} />,
    );

    const dialog = screen.getByRole("alertdialog", { name: "Reset environment?" });
    const description = dialog.querySelector('[data-slot="alert-dialog-description"]');
    expect(description).not.toBeNull();
    expect(description?.id).toBe(dialog.getAttribute("aria-describedby"));
    expect(description?.classList.contains("min-w-0")).toBe(true);
    expect(description?.classList.contains("text-left")).toBe(true);
    expect(description?.querySelectorAll("p")).toHaveLength(2);
    expect(description?.textContent).toContain("Any uncommitted or unpushed changes");
  });

  it("requires acknowledgment and passes the push-branch choice to confirmation", () => {
    const onConfirm = vi.fn();
    render(
      <TaskResetEnvConfirmDialog
        open
        onOpenChange={vi.fn()}
        hasWorktreePath
        onConfirm={onConfirm}
      />,
    );

    const confirm = screen.getByTestId("reset-env-confirm") as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);

    fireEvent.click(
      screen.getByRole("checkbox", {
        name: /push the current branch to its remote before resetting/i,
      }),
    );
    fireEvent.click(
      screen.getByRole("checkbox", {
        name: /I understand any uncommitted changes will be lost/i,
      }),
    );

    expect(confirm.disabled).toBe(false);
    fireEvent.click(confirm);
    expect(onConfirm).toHaveBeenCalledWith({ pushBranch: true });
  });

  it("locks both choices and confirmation while resetting", () => {
    render(
      <TaskResetEnvConfirmDialog
        open
        onOpenChange={vi.fn()}
        hasWorktreePath
        isResetting
        onConfirm={vi.fn()}
      />,
    );

    for (const checkbox of screen.getAllByRole("checkbox")) {
      expect((checkbox as HTMLButtonElement).disabled).toBe(true);
    }
    expect((screen.getByTestId("reset-env-confirm") as HTMLButtonElement).disabled).toBe(true);
  });
});
