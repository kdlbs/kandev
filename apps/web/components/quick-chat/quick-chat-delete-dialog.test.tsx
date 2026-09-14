import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";

import { QuickChatDeleteDialog } from "./quick-chat-delete-dialog";

afterEach(cleanup);

function renderDeleteDialog(onConfirm = vi.fn()) {
  return render(
    <QuickChatDeleteDialog sessionToDelete="chat-1" onOpenChange={vi.fn()} onConfirm={onConfirm} />,
  );
}

describe("QuickChatDeleteDialog structured description", () => {
  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.2
  it("keeps one left-aligned description boundary with semantic paragraphs and a list", () => {
    renderDeleteDialog();

    const dialog = screen.getByRole("alertdialog");
    const descriptions = dialog.querySelectorAll('[data-slot="alert-dialog-description"]');
    expect(descriptions).toHaveLength(1);

    const description = descriptions[0] as HTMLElement;
    expect(description.tagName).toBe("DIV");
    expect(description.className).toContain("min-w-0");
    expect(description.className).toContain("text-left");
    expect(description.className).toContain("space-y-2");
    expect(description.querySelectorAll(":scope > p")).toHaveLength(2);

    const list = description.querySelector(":scope > ul");
    expect(list).not.toBeNull();
    expect(list?.querySelectorAll(":scope > li")).toHaveLength(3);
  });

  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.5
  it("preserves the cancel and delete actions", async () => {
    const onConfirm = vi.fn();
    renderDeleteDialog(onConfirm);

    const dialog = screen.getByRole("alertdialog");
    expect(within(dialog).getByRole("button", { name: "Cancel" })).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
  });
});
