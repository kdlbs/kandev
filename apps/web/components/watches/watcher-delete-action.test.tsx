import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { WatcherDeleteAction } from "./watcher-delete-action";

afterEach(() => {
  cleanup();
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 1024 });
});
it("names the phone watch and preserves the trigger through cancellation", async () => {
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
  const onConfirm = vi.fn();
  render(
    <WatcherDeleteAction
      targetKey="watch-1"
      subject="ENG release bugs"
      title="Delete this watcher?"
      ariaLabel="Delete watcher"
      cancelLabel="Cancel"
      confirmLabel="Delete"
      onConfirm={onConfirm}
    />,
  );
  const trigger = screen.getByRole("button", { name: "Delete watcher" });
  fireEvent.click(trigger);
  const sheet = screen.getByRole("dialog", { name: "Delete this watcher?" });
  expect(within(sheet).getByText("ENG release bugs", { selector: "p" })).toBeTruthy();
  expect(trigger.isConnected).toBe(true);
  fireEvent.click(within(sheet).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(onConfirm).not.toHaveBeenCalled();
  fireEvent.click(trigger);
  fireEvent.click(screen.getByRole("button", { name: "Delete" }));
  await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
});
