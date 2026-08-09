import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { DeleteSessionDialog } from "./session-tab-menu";

describe("DeleteSessionDialog", () => {
  it("states conversation deletion and workspace retention", () => {
    render(
      <DeleteSessionDialog
        open
        onOpenChange={vi.fn()}
        isPrimary={false}
        sessionCount={1}
        onConfirm={vi.fn()}
      />,
    );

    const dialog = screen.getByRole("alertdialog");
    expect(dialog.textContent).toContain("permanently delete the conversation history");
    expect(dialog.textContent).toContain("task workspace and its files are kept");
    expect(dialog.textContent).toContain("only session for this task");
    // No uncommitted/unpushed warning belongs on session deletion.
    expect(dialog.textContent).not.toContain("uncommitted");
    expect(dialog.textContent).not.toContain("unpushed");
  });

  it("confirms deletion when the destructive action is activated", () => {
    const onConfirm = vi.fn();
    const onOpenChange = vi.fn();
    render(
      <DeleteSessionDialog
        open
        onOpenChange={onOpenChange}
        isPrimary={false}
        sessionCount={2}
        onConfirm={onConfirm}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
