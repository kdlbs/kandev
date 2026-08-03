import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PermanentDeleteDialog, QuarantinePurgeDialog } from "./storage-confirmation-dialogs";

afterEach(cleanup);

describe("storage confirmation dialogs", () => {
  // The confirm gate is `confirmation !== phrase`, so the phrase must reach the
  // user exactly as the comparison expects it. Translating it — or letting the
  // <Trans> tag index drift off its <strong> child — would leave a dialog the
  // user cannot satisfy, and nothing else in the suite would fail.
  it("shows the untranslated phrase and only enables the action on an exact match", () => {
    const onConfirm = vi.fn();
    render(
      <PermanentDeleteDialog entry={null} open onOpenChange={vi.fn()} onConfirm={onConfirm} />,
    );

    const description = screen.getByText(/This cannot be undone/);
    expect(description.textContent).toBe(
      "This cannot be undone. Kandev will permanently remove the selected quarantine entry. Type DELETE to continue.",
    );

    const input = screen.getByLabelText("Type DELETE to confirm");
    const action = screen.getByTestId("storage-quarantine-delete-confirm") as HTMLButtonElement;
    expect(action.disabled).toBe(true);

    fireEvent.change(input, { target: { value: "delete" } });
    expect(action.disabled).toBe(true);

    fireEvent.change(input, { target: { value: "DELETE" } });
    expect(action.disabled).toBe(false);
  });

  // Two independent counts cannot share one i18next `count`, so this sentence is
  // two plural messages joined. Both forms are asserted whole.
  it("agrees both quarantine counts with their own number", () => {
    const purge = (eligibleCount: number, protectedCount: number) =>
      render(
        <QuarantinePurgeDialog
          scope="eligible"
          eligibleCount={eligibleCount}
          protectedCount={protectedCount}
          open
          onOpenChange={vi.fn()}
          onConfirm={vi.fn()}
        />,
      );

    purge(1, 1);
    expect(screen.getByText(/This will permanently remove/).textContent).toBe(
      "This will permanently remove 1 eligible item. 1 protected item remains until its retention deadline. Type DELETE ELIGIBLE to continue.",
    );

    cleanup();
    purge(3, 2);
    expect(screen.getByText(/This will permanently remove/).textContent).toBe(
      "This will permanently remove 3 eligible items. 2 protected items remain until their retention deadlines. Type DELETE ELIGIBLE to continue.",
    );
  });
});
