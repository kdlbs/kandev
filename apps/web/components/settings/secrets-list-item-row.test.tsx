import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SecretListItem } from "@/lib/types/http-secrets";
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
  const onCopyMove = vi.fn();
  render(
    <SecretListItemRow
      secret={secret}
      onEdit={onEdit}
      onDelete={onDelete}
      onCopyMove={onCopyMove}
      isBusy={false}
      showCreate={false}
      isEditing={false}
      {...overrides}
    />,
  );
  return { onEdit, onDelete, onCopyMove };
}

afterEach(() => cleanup());

const COPY_MOVE_BUTTON = "Copy or move API Key";

/** Returns whether the copy/move button is currently disabled. */
function copyMoveDisabled(): boolean {
  return (screen.getByRole("button", { name: COPY_MOVE_BUTTON }) as HTMLButtonElement).disabled;
}

describe("SecretListItemRow", () => {
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
