import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { InlineConfirmActions } from "./inline-confirm-actions";

describe("InlineConfirmActions", () => {
  afterEach(cleanup);

  it("focuses Cancel first and gives touch actions a 44px active dimension", async () => {
    render(
      <InlineConfirmActions
        density="touch"
        cancelLabel="Cancel"
        confirmLabel="Delete"
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "Cancel" })),
    );
    expect(screen.getByRole("button", { name: "Delete" }).className).toContain("h-11");
    expect(screen.getByRole("button", { name: "Delete" }).className).toContain("min-w-11");
  });

  it("cancels on Escape without invoking the destructive action", () => {
    const onCancel = vi.fn();
    const onConfirm = vi.fn();
    render(
      <InlineConfirmActions
        cancelLabel="Cancel"
        confirmLabel="Delete"
        onCancel={onCancel}
        onConfirm={onConfirm}
      />,
    );

    fireEvent.keyDown(screen.getByRole("group"), { key: "Escape" });

    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("keeps mutation ownership with the consumer", async () => {
    const onConfirm = vi.fn(() => new Promise<void>(() => {}));
    render(
      <InlineConfirmActions
        cancelLabel="Cancel"
        confirmLabel="Delete"
        onCancel={vi.fn()}
        onConfirm={onConfirm}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
  });

  it("removes the shell before invoking the confirmation callback", async () => {
    let shellClosed = false;
    const onConfirm = vi.fn(() => {
      shellClosed = screen.queryByRole("group") === null;
    });
    render(
      <InlineConfirmActions
        cancelLabel="Cancel"
        confirmLabel="Delete"
        onCancel={vi.fn()}
        onConfirm={onConfirm}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(shellClosed).toBe(true);
  });
});
