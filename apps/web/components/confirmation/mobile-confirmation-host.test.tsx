import { useRef, useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MobileConfirmationHost, MobileConfirmationHostBody } from "./mobile-confirmation-host";
import { MobileActionConfirmation } from "./mobile-action-confirmation";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: true }),
}));
afterEach(cleanup);

function Harness({
  showSource = true,
  showTrigger = true,
  parentOpen = true,
  target = "A",
  onConfirm = vi.fn(),
  disabled = false,
  onCancel = vi.fn(),
}) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  return (
    <MobileConfirmationHost open={parentOpen}>
      {({ contentProps }) => (
        <div role="dialog" {...contentProps}>
          <MobileConfirmationHostBody>
            <input aria-label="Filter" defaultValue="unfinished draft" />
            <div data-testid="scroll-owner" style={{ overflowY: "auto", height: 100 }}>
              {showTrigger && (
                <button ref={trigger} onClick={() => setOpen(true)}>
                  Remove {target}
                </button>
              )}
            </div>
            {showSource && (
              <MobileActionConfirmation
                open={open}
                disabled={disabled}
                onCancel={onCancel}
                onOpenChange={setOpen}
                targetKey={target}
                title="Delete item?"
                subject={target}
                description="Deletes the selected item."
                cancelLabel="Cancel"
                confirmLabel="Delete"
                onConfirm={onConfirm}
                focusReturnRef={trigger}
              />
            )}
          </MobileConfirmationHostBody>
        </div>
      )}
    </MobileConfirmationHost>
  );
}

it("shows one modal step and restores the mounted draft, scroll and initiating focus", async () => {
  render(<Harness />);
  const input = screen.getByRole("textbox") as HTMLInputElement;
  const scroll = screen.getByTestId("scroll-owner");
  scroll.scrollTop = 48;
  fireEvent.change(input, { target: { value: "my filter" } });
  const trigger = screen.getByRole("button", { name: "Remove A" });
  fireEvent.click(trigger);
  expect(screen.getAllByRole("dialog")).toHaveLength(1);
  expect(screen.getByRole("dialog").getAttribute("aria-labelledby")).toBeTruthy();
  expect(screen.getByRole("group", { name: "Delete item?" }).closest("[inert]")).toBeNull();
  expect(screen.queryByRole("textbox")).toBeNull();
  expect(input.isConnected).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Back" }));
  expect(screen.getByRole("textbox")).toBe(input);
  expect(input.value).toBe("my filter");
  expect(scroll.scrollTop).toBe(48);
  await waitFor(() => expect(document.activeElement).toBe(trigger));
});

it("Escape cancels only the step without invoking the action", () => {
  const outsideKey = vi.fn();
  const onConfirm = vi.fn();
  render(
    <div onKeyDown={outsideKey}>
      <Harness onConfirm={onConfirm} />
    </div>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Remove A" }));
  fireEvent.keyDown(screen.getByRole("button", { name: "Cancel" }), { key: "Escape" });
  expect(screen.queryByRole("group")).toBeNull();
  expect(screen.getByRole("textbox")).toBeTruthy();
  expect(outsideKey).not.toHaveBeenCalled();
  expect(onConfirm).not.toHaveBeenCalled();
});

it("returns focus to a visible host control if its initiating control disappears", async () => {
  const { rerender } = render(<Harness />);
  fireEvent.click(screen.getByRole("button", { name: "Remove A" }));
  rerender(<Harness showTrigger={false} />);
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("textbox")));
});

it("invalidates the decision when its source unmounts", () => {
  const { rerender } = render(<Harness />);
  fireEvent.click(screen.getByRole("button", { name: "Remove A" }));
  expect(screen.getByRole("group")).toBeTruthy();
  rerender(<Harness showSource={false} />);
  expect(screen.queryByRole("group")).toBeNull();
});

it("does not resurrect a decision after closing and reopening its parent", () => {
  const { rerender } = render(<Harness />);
  fireEvent.click(screen.getByRole("button", { name: "Remove A" }));
  rerender(<Harness parentOpen={false} />);
  rerender(<Harness parentOpen />);
  expect(screen.queryByRole("group")).toBeNull();
});

it("invalidates even a disabled decision when its parent is dismissed", () => {
  const onCancel = vi.fn();
  const { rerender } = render(<Harness disabled onCancel={onCancel} />);
  fireEvent.click(screen.getByRole("button", { name: "Remove A" }));
  rerender(<Harness disabled parentOpen={false} onCancel={onCancel} />);
  expect(onCancel).toHaveBeenCalledOnce();
  rerender(<Harness disabled onCancel={onCancel} />);
  expect(screen.queryByRole("group")).toBeNull();
});

it("does not redirect a pending decision to a replacement target", () => {
  const onConfirm = vi.fn();
  const { rerender } = render(<Harness onConfirm={onConfirm} />);
  fireEvent.click(screen.getByRole("button", { name: "Remove A" }));
  expect(screen.getByRole("group")).toBeTruthy();
  rerender(<Harness target="B" onConfirm={onConfirm} />);
  expect(screen.queryByRole("group")).toBeNull();
  expect(onConfirm).not.toHaveBeenCalled();
});

it("lets a picker opt into one hosted step without changing its normal title or list", () => {
  function Picker() {
    const [open, setOpen] = useState(false);
    return (
      <MobilePickerSheet open onOpenChange={vi.fn()} title="Terminals" confirmationHost>
        <button onClick={() => setOpen(true)}>Close shell</button>
        <MobileActionConfirmation
          open={open}
          onOpenChange={setOpen}
          targetKey="shell"
          title="Close terminal?"
          subject="Shell"
          cancelLabel="Cancel"
          confirmLabel="Close"
          onConfirm={vi.fn()}
        />
      </MobilePickerSheet>
    );
  }
  render(<Picker />);
  const picker = screen.getByRole("dialog", { name: "Terminals" });
  fireEvent.click(screen.getByRole("button", { name: "Close shell" }));
  expect(screen.getAllByRole("dialog")).toHaveLength(1);
  expect(screen.getByRole("dialog", { name: "Close terminal?" })).toBe(picker);
  expect(screen.getAllByRole("dialog", { hidden: true })).toHaveLength(1);
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.getByRole("dialog", { name: "Terminals" })).toBeTruthy();
  expect(screen.queryByRole("group")).toBeNull();
});

it("an older request's cleanup cannot remove the replacement request", () => {
  function Requests({ second }: { second: boolean }) {
    const [first, setFirst] = useState(true);
    return (
      <MobileConfirmationHost open>
        {({ contentProps }) => (
          <div role="dialog" {...contentProps}>
            <MobileConfirmationHostBody>
              <MobileActionConfirmation
                open={first}
                onOpenChange={setFirst}
                targetKey="first"
                title="First?"
                cancelLabel="Cancel"
                confirmLabel="Delete"
                onConfirm={vi.fn()}
              />
              <MobileActionConfirmation
                open={second}
                onOpenChange={vi.fn()}
                targetKey="second"
                title="Second?"
                cancelLabel="Cancel"
                confirmLabel="Delete"
                onConfirm={vi.fn()}
              />
            </MobileConfirmationHostBody>
          </div>
        )}
      </MobileConfirmationHost>
    );
  }
  const { rerender } = render(<Requests second={false} />);
  expect(screen.getByRole("group", { name: "First?" })).toBeTruthy();
  rerender(<Requests second />);
  expect(screen.getByRole("group", { name: "Second?" })).toBeTruthy();
  expect(screen.queryByRole("group", { name: "First?" })).toBeNull();
});
