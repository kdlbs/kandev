import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useRef, useState } from "react";

import { ContextMenu, ContextMenuTrigger } from "@kandev/ui/context-menu";

import {
  DeleteSessionDialog,
  DeleteSessionPopover,
  SessionContextMenuItems,
} from "./session-tab-menu";

describe("DeleteSessionDialog", () => {
  it("states conversation deletion and workspace retention", () => {
    render(
      <DeleteSessionDialog
        open
        onOpenChange={vi.fn()}
        isPrimary={false}
        sessionCount={1}
        onConfirm={vi.fn()}
      />,
    );

    const dialog = screen.getByRole("alertdialog");
    expect(dialog.textContent).toContain("permanently delete the conversation history");
    expect(dialog.textContent).toContain("task workspace and its files are kept");
    expect(dialog.textContent).toContain("only session for this task");
    // No uncommitted/unpushed warning belongs on session deletion.
    expect(dialog.textContent).not.toContain("uncommitted");
    expect(dialog.textContent).not.toContain("unpushed");
  });

  it("confirms deletion when the destructive action is activated", () => {
    const onConfirm = vi.fn();
    const onOpenChange = vi.fn();
    render(
      <DeleteSessionDialog
        open
        onOpenChange={onOpenChange}
        isPrimary={false}
        sessionCount={2}
        onConfirm={onConfirm}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

afterEach(() => {
  cleanup();
});

describe("SessionContextMenuItems", () => {
  it("keeps the delete menu action open for its local confirmation", () => {
    const onDelete = vi.fn<(event: Event) => void>();
    render(
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <button type="button">Session</button>
        </ContextMenuTrigger>
        <SessionContextMenuItems
          sessionState="COMPLETED"
          isPrimary={false}
          canShare={false}
          taskId={null}
          sessionId={undefined}
          actions={{
            handleSetPrimary: vi.fn(),
            handleStop: vi.fn(),
            handleResume: vi.fn(),
            handleCloseOthers: vi.fn(),
          }}
          onDelete={onDelete}
          onShare={vi.fn()}
          onHandoffProfile={vi.fn()}
          onStartRename={vi.fn()}
        />
      </ContextMenu>,
    );

    fireEvent.contextMenu(screen.getByRole("button", { name: "Session" }), {
      clientX: 100,
      clientY: 100,
    });
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));

    expect(onDelete).toHaveBeenCalledTimes(1);
    expect(onDelete.mock.calls[0]?.[0].defaultPrevented).toBe(true);
  });
});

function SessionDeletePopoverHarness({ onConfirm }: { onConfirm: () => void }) {
  const [open, setOpen] = useState(true);
  const anchorRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      <button ref={anchorRef} type="button">
        Delete session
      </button>
      <DeleteSessionPopover
        open={open}
        anchorRef={anchorRef}
        onOpenChange={setOpen}
        isPrimary
        sessionCount={2}
        targetName="Mock Fast"
        onConfirm={onConfirm}
      />
    </>
  );
}

describe("DeleteSessionPopover", () => {
  it("cancels locally without dispatching an alert dialog", async () => {
    const onConfirm = vi.fn();
    render(<SessionDeletePopoverHarness onConfirm={onConfirm} />);

    const popover = await screen.findByTestId("session-delete-confirm-popover");
    expect(screen.queryByRole("alertdialog")).toBeNull();
    fireEvent.click(within(popover).getByRole("button", { name: "Cancel" }));

    expect(onConfirm).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByTestId("session-delete-confirm-popover")).toBeNull());
  });

  it("confirms once after closing the local shell", async () => {
    let shellClosed = false;
    const onConfirm = vi.fn(() => {
      shellClosed = screen.queryByTestId("session-delete-confirm-popover") === null;
    });
    render(<SessionDeletePopoverHarness onConfirm={onConfirm} />);

    fireEvent.click(screen.getByTestId("session-delete-confirm"));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(shellClosed).toBe(true);
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });
});
