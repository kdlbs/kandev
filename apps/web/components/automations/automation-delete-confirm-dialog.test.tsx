import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AutomationDeleteConfirmDialog } from "./automation-delete-confirm-dialog";

afterEach(cleanup);

describe("AutomationDeleteConfirmDialog", () => {
  it("keeps the controlled dialog open while confirmation is pending", () => {
    const onConfirm = vi.fn(() => new Promise<void>(() => {}));
    const onOpenChange = vi.fn();

    render(
      <AutomationDeleteConfirmDialog
        open
        automationName="Nightly cleanup"
        onOpenChange={onOpenChange}
        onConfirm={onConfirm}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(screen.getByRole("alertdialog")).toBeTruthy();
  });
});
