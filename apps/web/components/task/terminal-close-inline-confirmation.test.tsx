import { useRef, useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { TerminalCloseInlineConfirmation } from "./terminal-close-inline-confirmation";
import { MobilePickerSheet } from "./mobile/mobile-picker-sheet";

beforeEach(() => Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 }));
afterEach(() => {
  cleanup();
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 1024 });
});

function Picker({ onConfirm, events }: { onConfirm: () => Promise<void>; events: string[] }) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  return (
    <MobilePickerSheet open onOpenChange={vi.fn()} title="Terminals" confirmationHost>
      <button ref={trigger} onClick={() => setOpen(true)}>
        Close build shell
      </button>
      <button>Sibling shell</button>
      {open && (
        <TerminalCloseInlineConfirmation
          terminalId="shell-build"
          terminalLabel="Build shell"
          focusReturnRef={trigger}
          onCancel={() => {
            events.push("cancel");
            setOpen(false);
          }}
          onClose={() => {
            events.push("close");
            setOpen(false);
          }}
          onConfirm={onConfirm}
        />
      )}
    </MobilePickerSheet>
  );
}

it("closes its named picker step before teardown and permits only one dispatch", async () => {
  const events: string[] = [];
  let finish!: () => void;
  const onConfirm = vi.fn(() => {
    events.push("dispatch");
    return new Promise<void>((resolve) => {
      finish = resolve;
    });
  });
  render(<Picker onConfirm={onConfirm} events={events} />);
  const picker = screen.getByRole("dialog", { name: "Terminals" });
  fireEvent.click(screen.getByRole("button", { name: "Close build shell" }));
  expect(screen.getByRole("dialog", { name: "Close terminal?" })).toBe(picker);
  expect(screen.getByRole("group").textContent).toContain("Build shell");
  expect(screen.queryByRole("button", { name: "Sibling shell" })).toBeNull();
  const confirm = screen.getByRole("button", { name: "Close terminal" });
  act(() => {
    fireEvent.click(confirm);
    fireEvent.click(confirm);
  });
  expect(screen.getByRole("button", { name: "Sibling shell" })).toBeTruthy();
  await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce());
  expect(events).toEqual(["close", "dispatch"]);
  await act(async () => finish());
});

it("Back restores the original trigger without calling close or teardown", async () => {
  const events: string[] = [];
  const onConfirm = vi.fn().mockResolvedValue(undefined);
  render(<Picker onConfirm={onConfirm} events={events} />);
  const trigger = screen.getByRole("button", { name: "Close build shell" });
  fireEvent.click(trigger);
  fireEvent.click(screen.getByRole("button", { name: "Back" }));
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(events).toEqual(["cancel"]);
  expect(onConfirm).not.toHaveBeenCalled();
});
