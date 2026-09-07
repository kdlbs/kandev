import { useRef, useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { isActionConfirmationTarget } from "./action-confirm-popover";
import { SavedTaskViewDeleteConfirmation } from "./saved-task-view-delete-confirmation";

const TARGET = { id: "view-1", label: "Needs review" };
const TARGET_TITLE = "Delete Needs review?";

function Harness({
  presentation,
  onConfirm,
  confirmDisabled = false,
}: {
  presentation: "popover" | "inline";
  onConfirm: (id: string) => void;
  confirmDisabled?: boolean;
}) {
  const [open, setOpen] = useState(true);
  const anchorRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      <button ref={anchorRef} type="button" data-testid="delete-anchor">
        Delete view
      </button>
      <SavedTaskViewDeleteConfirmation
        target={TARGET}
        presentation={presentation}
        open={open}
        anchorRef={anchorRef}
        confirmDisabled={confirmDisabled}
        onOpenChange={setOpen}
        onConfirm={onConfirm}
      />
    </>
  );
}

function ReplacingHarness() {
  const [open, setOpen] = useState(false);
  const anchorRef = useRef<HTMLButtonElement>(null);

  if (!open) {
    return (
      <button
        ref={anchorRef}
        type="button"
        data-testid="replacement-delete-anchor"
        onClick={() => setOpen(true)}
      >
        Delete view
      </button>
    );
  }

  return (
    <SavedTaskViewDeleteConfirmation
      target={TARGET}
      presentation="inline"
      open
      anchorRef={anchorRef}
      onOpenChange={setOpen}
      onConfirm={vi.fn()}
    />
  );
}

describe("SavedTaskViewDeleteConfirmation", () => {
  afterEach(cleanup);

  it("names the target and dispatches its captured id only after fine-pointer confirmation", async () => {
    const onConfirm = vi.fn();
    render(<Harness presentation="popover" onConfirm={onConfirm} />);

    const dialog = screen.getByRole("dialog", { name: TARGET_TITLE });
    expect(dialog.textContent).toContain(
      "This removes only the saved filters and view settings. It does not delete tasks or data in connected services.",
    );
    expect(isActionConfirmationTarget(dialog)).toBe(true);
    expect(onConfirm).not.toHaveBeenCalled();

    fireEvent.click(within(dialog).getByRole("button", { name: "Delete Needs review" }));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce());
    expect(onConfirm).toHaveBeenCalledWith("view-1");
  });

  it("cancels the inline presentation on Escape and returns focus to the trigger", async () => {
    const onConfirm = vi.fn();
    render(<Harness presentation="inline" onConfirm={onConfirm} />);

    const confirmation = screen.getByRole("group", { name: TARGET_TITLE });
    expect(confirmation.textContent).toContain(TARGET_TITLE);
    fireEvent.keyDown(confirmation, { key: "Escape" });

    expect(onConfirm).not.toHaveBeenCalled();
    expect(screen.queryByRole("group", { name: TARGET_TITLE })).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId("delete-anchor")));
  });

  it("returns focus after inline confirmation is replaced by its trigger", async () => {
    render(<ReplacingHarness />);

    fireEvent.click(screen.getByTestId("replacement-delete-anchor"));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    const replacementAnchor = await screen.findByTestId("replacement-delete-anchor");
    await waitFor(() => expect(document.activeElement).toBe(replacementAnchor));
  });

  it("keeps Cancel available while inline confirmation is disabled", () => {
    render(<Harness presentation="inline" onConfirm={vi.fn()} confirmDisabled />);

    expect(screen.getByRole("button", { name: "Cancel" }).hasAttribute("disabled")).toBe(false);
    expect(
      screen.getByRole("button", { name: "Delete Needs review" }).hasAttribute("disabled"),
    ).toBe(true);
  });
});
