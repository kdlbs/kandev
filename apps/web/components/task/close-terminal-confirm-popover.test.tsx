import { useRef, useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CloseTerminalConfirmPopover } from "./close-terminal-confirm-popover";

function deferredPromise() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("CloseTerminalConfirmPopover", () => {
  afterEach(cleanup);

  it("uses a localized popover and dismisses before terminal teardown settles", async () => {
    const teardown = deferredPromise();
    const onConfirm = vi.fn(() => teardown.promise);

    function Harness() {
      const [open, setOpen] = useState(true);
      const anchorRef = useRef<HTMLButtonElement>(null);
      return (
        <>
          <button ref={anchorRef} type="button">
            close
          </button>
          <CloseTerminalConfirmPopover
            open={open}
            terminalName="Terminal"
            anchorRef={anchorRef}
            onOpenChange={setOpen}
            onConfirm={onConfirm}
          />
        </>
      );
    }

    render(<Harness />);
    expect(screen.getByRole("dialog", { name: "Close terminal?" })).toBeTruthy();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(screen.getByRole("button", { name: "Close terminal" }).className).toContain("min-h-11");

    fireEvent.click(screen.getByRole("button", { name: "Close terminal" }));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole("dialog", { name: "Close terminal?" })).toBeNull();
  });
});
