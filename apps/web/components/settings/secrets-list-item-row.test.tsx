import { cleanup, render, screen } from "@testing-library/react";
import { fireEvent, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SecretListItem } from "@/lib/types/http-secrets";

let finePointer = true;

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: finePointer }),
}));

import { SecretListItemRow } from "./secrets-list-item-row";

const secret: SecretListItem = {
  id: "secret-1",
  name: "API Key",
  scope: "global",
  has_value: true,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

/** Renders the row with test fakes and returns the spies. */
function renderRow(overrides: Partial<Parameters<typeof SecretListItemRow>[0]> = {}) {
  const onEdit = vi.fn();
  const onDelete = vi.fn();
  const onDeleteCancel = vi.fn();
  const onDeleteConfirm = vi.fn();
  const onCopyMove = vi.fn();
  render(
    <SecretListItemRow
      secret={secret}
      onEdit={onEdit}
      onDelete={onDelete}
      onDeleteCancel={onDeleteCancel}
      onDeleteConfirm={onDeleteConfirm}
      onCopyMove={onCopyMove}
      isBusy={false}
      showCreate={false}
      isEditing={false}
      isDeleteConfirming={false}
      {...overrides}
    />,
  );
  return { onEdit, onDelete, onDeleteCancel, onDeleteConfirm, onCopyMove };
}

afterEach(() => {
  finePointer = true;
  cleanup();
});

const COPY_MOVE_BUTTON = "Copy or move API Key";
const DELETE_BUTTON = "Delete secret API Key";
const CONFIRM_POPOVER_TEST_ID = "secret-delete-confirm-popover";
const CONFIRM_TEST_ID = "secret-delete-confirm";

/** Returns whether the copy/move button is currently disabled. */
function copyMoveDisabled(): boolean {
  return (screen.getByRole("button", { name: COPY_MOVE_BUTTON }) as HTMLButtonElement).disabled;
}

describe("SecretListItemRow", () => {
  it("keeps delete trigger target-aware and touch-sized", () => {
    const { onDelete } = renderRow();
    const deleteButton = screen.getByRole("button", { name: DELETE_BUTTON });

    expect(deleteButton.className).toContain("min-h-11");
    expect(deleteButton.className).toContain("min-w-11");
    fireEvent.click(deleteButton);

    expect(onDelete).toHaveBeenCalledWith(secret);
  });

  it("anchors fine-pointer confirmation to this row", async () => {
    const { onDeleteConfirm } = renderRow({ isDeleteConfirming: true });
    const popover = screen.getByTestId(CONFIRM_POPOVER_TEST_ID);

    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(within(popover).getByRole("button", { name: DELETE_BUTTON })).not.toBeNull();
    fireEvent.click(within(popover).getByTestId(CONFIRM_TEST_ID));

    await waitFor(() => expect(onDeleteConfirm).toHaveBeenCalledTimes(1));
  });

  it("morphs into inline touch confirmation on coarse pointers", async () => {
    finePointer = false;
    const { onDeleteCancel, onDeleteConfirm } = renderRow({ isDeleteConfirming: true });
    const inline = screen.getByTestId("secret-delete-inline-confirmation");

    expect(screen.queryByTestId(CONFIRM_POPOVER_TEST_ID)).toBeNull();
    expect(screen.queryByRole("button", { name: COPY_MOVE_BUTTON })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reveal secret API Key" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit secret API Key" })).toBeNull();
    expect(inline.getAttribute("aria-label")).toBe(DELETE_BUTTON);
    expect(within(inline).getByTestId(CONFIRM_TEST_ID).className).toContain("h-11");
    expect(within(inline).getByTestId(CONFIRM_TEST_ID).className).toContain("min-w-11");

    fireEvent.click(within(inline).getByRole("button", { name: "Cancel" }));
    expect(onDeleteCancel).toHaveBeenCalledTimes(1);
    fireEvent.click(within(inline).getByTestId(CONFIRM_TEST_ID));
    await waitFor(() => expect(onDeleteConfirm).toHaveBeenCalledTimes(1));
  });

  it("runs the copy/move action", () => {
    const { onCopyMove } = renderRow();
    screen.getByRole("button", { name: COPY_MOVE_BUTTON }).click();
    expect(onCopyMove).toHaveBeenCalledWith(secret);
  });

  it("disables copy/move while a create draft is open", () => {
    renderRow({ showCreate: true });
    expect(copyMoveDisabled()).toBe(true);
  });

  it("disables copy/move while that row is being edited", () => {
    renderRow({ isEditing: true });
    expect(copyMoveDisabled()).toBe(true);
  });

  it("disables copy/move while a transfer is busy", () => {
    renderRow({ isBusy: true });
    expect(copyMoveDisabled()).toBe(true);
  });
});
