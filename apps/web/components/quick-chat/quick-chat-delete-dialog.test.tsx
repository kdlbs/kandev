import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";

import { QuickChatDeleteDialog } from "./quick-chat-delete-dialog";

const DISCARD_CHECKBOX = "delete-discard-worktree-checkbox";
const preview = vi.hoisted(() => ({ dirty: false, status: "resolved", inspection: "complete" }));
vi.mock("@/hooks/use-task-delete-preflight", () => ({
  useTaskDeletePreflight: () => ({
    status: preview.status,
    requiresDiscardConsent: preview.dirty,
    confirmationId: "preview-1",
    workspaceInspection: preview.inspection,
    requestKey: "task-1",
    scopeKey: "task-1",
    retry: vi.fn(),
  }),
}));

afterEach(cleanup);
beforeEach(() => {
  preview.dirty = false;
  preview.status = "resolved";
  preview.inspection = "complete";
});

function renderDeleteDialog(onConfirm = vi.fn()) {
  return render(
    <QuickChatDeleteDialog
      sessionToDelete="chat-1"
      taskId="task-1"
      onOpenChange={vi.fn()}
      onConfirm={onConfirm}
    />,
  );
}

describe("QuickChatDeleteDialog structured description", () => {
  // @covers AC-UI-TASK-CLEANUP-CONFIRMATION-001.12
  it("requires explicit consent before deleting a dirty worktree", () => {
    preview.dirty = true;
    renderDeleteDialog();
    const dialog = screen.getByRole("alertdialog");
    const checkbox = within(dialog).getByTestId(DISCARD_CHECKBOX);
    const deleteButton = within(dialog).getByRole("button", { name: "Delete" });
    expect((checkbox as HTMLButtonElement).getAttribute("aria-checked")).toBe("false");
    expect((deleteButton as HTMLButtonElement).disabled).toBe(true);
  });

  it("submits the consent and matching preview after selection", async () => {
    preview.dirty = true;
    const onConfirm = vi.fn();
    renderDeleteDialog(onConfirm);
    const dialog = screen.getByRole("alertdialog");
    fireEvent.click(within(dialog).getByTestId(DISCARD_CHECKBOX));
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() =>
      expect(onConfirm).toHaveBeenCalledWith({
        confirmationId: "preview-1",
        discardWorktreeChanges: true,
      }),
    );
  });

  it("blocks deletion when inspection fails and offers retry", () => {
    preview.status = "error";
    renderDeleteDialog();
    const dialog = screen.getByRole("alertdialog");
    expect(within(dialog).getByTestId("delete-preflight-error")).toBeTruthy();
    expect(
      (within(dialog).getByRole("button", { name: "Delete" }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  it("requires explicit acknowledgement when inspection is unknown", async () => {
    preview.dirty = true;
    preview.inspection = "unknown";
    const onConfirm = vi.fn();
    renderDeleteDialog(onConfirm);
    const dialog = screen.getByRole("alertdialog");
    expect(
      (within(dialog).getByRole("button", { name: "Delete" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    fireEvent.click(within(dialog).getByTestId("delete-unverified-workspace-checkbox"));
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() =>
      expect(onConfirm).toHaveBeenCalledWith({
        confirmationId: "preview-1",
        discardWorktreeChanges: true,
        allowUnverifiedWorkspaceChanges: true,
      }),
    );
  });

  it("resets discard consent when the target changes", () => {
    preview.dirty = true;
    const rendered = renderDeleteDialog();
    fireEvent.click(screen.getByTestId(DISCARD_CHECKBOX));
    rendered.rerender(
      <QuickChatDeleteDialog
        sessionToDelete="chat-2"
        taskId="task-2"
        onOpenChange={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );
    expect(screen.getByTestId(DISCARD_CHECKBOX).getAttribute("aria-checked")).toBe("false");
    expect((screen.getByRole("button", { name: "Delete" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

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
