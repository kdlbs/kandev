import { useRef, useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ActionConfirmPopover } from "./action-confirm-popover";

const deleteTitle = "Delete item?";

function Harness({
  onConfirm = vi.fn(),
  confirmDisabled = false,
  size,
}: {
  onConfirm?: () => void | Promise<void>;
  confirmDisabled?: boolean;
  size?: "wide";
}) {
  const [open, setOpen] = useState(true);
  const [showAnchor, setShowAnchor] = useState(true);
  const anchorRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      {showAnchor ? (
        <button ref={anchorRef} type="button" data-testid="anchor" onClick={() => setOpen(true)}>
          Delete
        </button>
      ) : null}
      <button type="button" onClick={() => setShowAnchor(false)}>
        Remove anchor
      </button>
      <ActionConfirmPopover
        open={open}
        anchorRef={anchorRef}
        title={deleteTitle}
        description="This cannot be undone."
        cancelLabel="Cancel"
        confirmLabel="Delete"
        confirmDisabled={confirmDisabled}
        size={size}
        onOpenChange={setOpen}
        onConfirm={onConfirm}
      />
    </>
  );
}

describe("ActionConfirmPopover", () => {
  afterEach(cleanup);

  it("focuses Cancel first and gives coarse pointers a touch-sized action", async () => {
    render(<Harness />);

    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "Cancel" })),
    );
    expect(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }).className,
    ).toContain("min-h-11");
  });

  it("supports a wider bounded surface without changing the default width", async () => {
    render(<Harness />);
    expect(screen.getByRole("dialog").className).toContain("w-64");

    cleanup();
    render(<Harness size="wide" />);
    const wideDialog = screen.getByRole("dialog");
    expect(wideDialog.className).toContain("w-72");
    expect(wideDialog.className).toContain("max-w-[calc(100vw-1rem)]");
  });

  it("closes before invoking an unresolved confirmation and returns focus on cancel", async () => {
    const onConfirm = vi.fn(() => new Promise<void>(() => {}));
    render(<Harness onConfirm={onConfirm} />);

    const cancel = screen.getByRole("button", { name: "Cancel" });
    const anchor = screen.getByTestId("anchor");
    fireEvent.click(cancel);

    expect(onConfirm).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog", { name: deleteTitle })).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(anchor));
  });

  it("removes the shell before invoking the confirmation callback", async () => {
    let shellClosed = false;
    const onConfirm = vi.fn(() => {
      shellClosed = screen.queryByRole("dialog", { name: deleteTitle }) === null;
    });
    render(<Harness onConfirm={onConfirm} />);

    fireEvent.click(
      within(screen.getByRole("dialog", { name: deleteTitle })).getByRole("button", {
        name: "Delete",
      }),
    );

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(shellClosed).toBe(true);
  });

  it("does not restore focus to the confirmed action anchor", async () => {
    const onConfirm = vi.fn();
    render(<Harness onConfirm={onConfirm} />);

    const anchor = screen.getByTestId("anchor");
    fireEvent.click(
      within(screen.getByRole("dialog", { name: deleteTitle })).getByRole("button", {
        name: "Delete",
      }),
    );

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(document.activeElement).not.toBe(anchor);
  });

  it("keeps a disabled confirmation inert", () => {
    const onConfirm = vi.fn();
    render(<Harness onConfirm={onConfirm} confirmDisabled />);

    const confirm = within(screen.getByRole("dialog", { name: deleteTitle })).getByRole("button", {
      name: "Delete",
    });
    expect(confirm.hasAttribute("disabled")).toBe(true);
    expect(screen.getByRole("button", { name: "Cancel" }).hasAttribute("disabled")).toBe(false);
    fireEvent.click(confirm);

    expect(onConfirm).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog", { name: deleteTitle })).toBeTruthy();
  });

  it("handles a rejected callback without an unhandled rejection", async () => {
    const unhandled = vi.fn();
    const onUnhandled = (event: PromiseRejectionEvent) => {
      event.preventDefault();
      unhandled(event.reason);
    };
    window.addEventListener("unhandledrejection", onUnhandled);
    const onConfirm = vi.fn(() => Promise.reject(new Error("delete failed")));
    render(<Harness onConfirm={onConfirm} />);

    fireEvent.click(
      within(screen.getByRole("dialog", { name: deleteTitle })).getByRole("button", {
        name: "Delete",
      }),
    );

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(unhandled).not.toHaveBeenCalled();
    window.removeEventListener("unhandledrejection", onUnhandled);
  });
});

describe("ActionConfirmPopover anchor lifecycle", () => {
  afterEach(cleanup);

  it("dismisses without confirming when the anchor disappears", async () => {
    function DisappearingHarness() {
      const [open, setOpen] = useState(true);
      const [showAnchor, setShowAnchor] = useState(true);
      const anchorRef = useRef<HTMLButtonElement>(null);
      const onConfirm = vi.fn();
      return (
        <>
          {showAnchor ? (
            <button ref={anchorRef} type="button" onClick={() => setShowAnchor(false)}>
              Remove target
            </button>
          ) : null}
          <ActionConfirmPopover
            open={open}
            anchorRef={anchorRef}
            title={deleteTitle}
            description="This cannot be undone."
            cancelLabel="Cancel"
            confirmLabel="Delete"
            onOpenChange={setOpen}
            onConfirm={onConfirm}
          />
        </>
      );
    }

    render(<DisappearingHarness />);
    fireEvent.click(screen.getByRole("button", { name: "Remove target" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("returns focus to a separate focus target on cancel", async () => {
    function SeparateFocusHarness() {
      const [open, setOpen] = useState(true);
      const anchorRef = useRef<HTMLDivElement>(null);
      const focusReturnRef = useRef<HTMLButtonElement>(null);
      return (
        <>
          <div ref={anchorRef}>Positioning anchor</div>
          <button ref={focusReturnRef} type="button">
            More options
          </button>
          <ActionConfirmPopover
            open={open}
            anchorRef={anchorRef}
            focusReturnRef={focusReturnRef}
            title={deleteTitle}
            description="This cannot be undone."
            cancelLabel="Cancel"
            confirmLabel="Delete"
            onOpenChange={setOpen}
            onConfirm={vi.fn()}
          />
        </>
      );
    }

    render(<SeparateFocusHarness />);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "More options" })),
    );
  });
});

function BoundaryHarness() {
  const [open, setOpen] = useState(true);
  const anchorRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  return (
    <>
      <button ref={anchorRef} type="button">
        Delete
      </button>
      <div ref={menuRef} data-testid="menu-boundary" tabIndex={0}>
        Menu
      </div>
      <ActionConfirmPopover
        open={open}
        anchorRef={anchorRef}
        focusBoundaryRef={menuRef}
        title={deleteTitle}
        description="This cannot be undone."
        cancelLabel="Cancel"
        confirmLabel="Delete"
        onOpenChange={setOpen}
        onConfirm={vi.fn()}
      />
    </>
  );
}

describe("ActionConfirmPopover focus boundaries", () => {
  afterEach(cleanup);

  it("keeps the popover open while focus moves inside the optional menu boundary", async () => {
    render(<BoundaryHarness />);
    screen.getByTestId("menu-boundary").focus();

    await waitFor(() => expect(screen.getByRole("dialog", { name: deleteTitle })).toBeTruthy());
  });

  it("keeps the popover open while pointer interaction stays inside the optional boundary", async () => {
    render(<BoundaryHarness />);
    fireEvent.pointerDown(screen.getByTestId("menu-boundary"));

    await waitFor(() => expect(screen.getByRole("dialog", { name: deleteTitle })).toBeTruthy());
  });

  it("dismisses when focus moves outside the optional menu boundary", async () => {
    function MenuHarness() {
      const [open, setOpen] = useState(true);
      const anchorRef = useRef<HTMLButtonElement>(null);
      const menuRef = useRef<HTMLDivElement>(null);
      return (
        <>
          <button ref={anchorRef} type="button">
            Delete
          </button>
          <div ref={menuRef} data-testid="menu-boundary" tabIndex={0}>
            Menu
          </div>
          <button type="button" data-testid="outside">
            Outside
          </button>
          <ActionConfirmPopover
            open={open}
            anchorRef={anchorRef}
            focusBoundaryRef={menuRef}
            title={deleteTitle}
            description="This cannot be undone."
            cancelLabel="Cancel"
            confirmLabel="Delete"
            onOpenChange={setOpen}
            onConfirm={vi.fn()}
          />
        </>
      );
    }

    render(<MenuHarness />);
    screen.getByTestId("outside").focus();

    await waitFor(() => expect(screen.queryByRole("dialog", { name: deleteTitle })).toBeNull());
  });
});
