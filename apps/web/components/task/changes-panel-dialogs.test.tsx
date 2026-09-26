import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useRef, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/components/confirmation/action-confirm-popover", () => ({
  ActionConfirmPopover: ({ onConfirm }: { onConfirm: () => void }) => (
    <div data-testid="discard-local-confirmation">
      <button type="button" onClick={onConfirm}>
        confirm
      </button>
    </div>
  ),
}));

import { DiscardDialog } from "./changes-panel-dialogs";

afterEach(cleanup);

function DiscardHarness({
  fileToDiscard,
  filesToDiscard,
}: {
  fileToDiscard: string | null;
  filesToDiscard: string[] | null;
}) {
  const [open, setOpen] = useState(true);
  const anchorRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      <button ref={anchorRef} type="button">
        discard
      </button>
      <DiscardDialog
        open={open}
        onOpenChange={setOpen}
        fileToDiscard={fileToDiscard}
        filesToDiscard={filesToDiscard}
        anchorRef={anchorRef}
        onConfirm={vi.fn()}
      />
    </>
  );
}

describe("DiscardDialog target scope", () => {
  it.each([
    ["direct file", "src/one.ts", null],
    ["one selected file", null, ["src/one.ts"]],
  ] as const)("uses local confirmation for a %s", (_kind, fileToDiscard, filesToDiscard) => {
    render(
      <DiscardHarness
        fileToDiscard={fileToDiscard}
        filesToDiscard={filesToDiscard ? [...filesToDiscard] : null}
      />,
    );

    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(screen.getByTestId("discard-local-confirmation")).toBeTruthy();
  });

  it("keeps discard-all scope in the existing modal", () => {
    render(<DiscardHarness fileToDiscard={null} filesToDiscard={["src/one.ts", "src/two.ts"]} />);

    expect(screen.getByRole("alertdialog")).toBeTruthy();
    expect(screen.queryByTestId("discard-local-confirmation")).toBeNull();
  });

  it("does not invoke confirmation mutation before local confirm", () => {
    const onConfirm = vi.fn();
    function Harness() {
      const [open, setOpen] = useState(true);
      const anchorRef = useRef<HTMLButtonElement>(null);
      return (
        <>
          <button ref={anchorRef} type="button">
            discard
          </button>
          <DiscardDialog
            open={open}
            onOpenChange={setOpen}
            fileToDiscard="src/one.ts"
            filesToDiscard={null}
            anchorRef={anchorRef}
            onConfirm={onConfirm}
          />
        </>
      );
    }

    render(<Harness />);
    expect(onConfirm).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "confirm" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});
