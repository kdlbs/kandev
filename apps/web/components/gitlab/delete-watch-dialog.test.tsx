import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DeleteWatchDialog } from "./delete-watch-dialog";

afterEach(cleanup);

describe("DeleteWatchDialog", () => {
  it("warns that every task created by the watch is deleted before confirming", async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined);
    const onOpenChange = vi.fn();
    render(
      <DeleteWatchDialog
        open
        onOpenChange={onOpenChange}
        watchLabel="GitLab review watch"
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByText(/delete every task created by this watch/i)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Delete watch" }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("keeps the dialog open and exposes a retryable error when deletion fails", async () => {
    const onConfirm = vi.fn().mockRejectedValue(new Error("Deletion blocked"));
    render(
      <DeleteWatchDialog
        open
        onOpenChange={vi.fn()}
        watchLabel="GitLab issue watch"
        onConfirm={onConfirm}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Delete watch" }));
    expect(await screen.findByText("Deletion blocked")).toBeTruthy();
    expect(
      (screen.getByRole("button", { name: "Delete watch" }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });
});
