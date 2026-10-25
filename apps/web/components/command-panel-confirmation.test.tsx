import { useRef, useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CommandPanelDialog } from "./command-panel-dialog";
import { MobileActionConfirmation } from "./confirmation/mobile-action-confirmation";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: true }),
}));
afterEach(cleanup);

it("keeps the command dialog and its query while presenting one explicit decision step", () => {
  const onConfirm = vi.fn();
  const onKeyDown = vi.fn();
  function Panel() {
    const [open, setOpen] = useState(false);
    const trigger = useRef<HTMLButtonElement>(null);
    return (
      <CommandPanelDialog open>
        <div onKeyDown={onKeyDown}>
          <input aria-label="Query" defaultValue="archive" />
          <button ref={trigger} onClick={() => setOpen(true)}>
            Archive command
          </button>
          <MobileActionConfirmation
            open={open}
            onOpenChange={setOpen}
            targetKey="task"
            title="Archive task?"
            subject="Selected task"
            cancelLabel="Cancel"
            confirmLabel="Archive"
            variant="default"
            onConfirm={onConfirm}
            focusReturnRef={trigger}
          />
        </div>
      </CommandPanelDialog>
    );
  }
  render(<Panel />);
  const dialog = screen.getByRole("dialog");
  const query = screen.getByRole("textbox");
  fireEvent.change(query, { target: { value: "my query" } });
  fireEvent.click(screen.getByRole("button", { name: "Archive command" }));
  expect(screen.getByRole("dialog", { name: "Archive task?" })).toBe(dialog);
  expect(screen.getAllByRole("dialog", { hidden: true })).toHaveLength(1);
  expect(screen.getAllByRole("heading")).toHaveLength(1);
  expect(screen.queryByRole("textbox")).toBeNull();
  fireEvent.keyDown(screen.getByRole("group"), { key: "Enter" });
  expect(onConfirm).not.toHaveBeenCalled();
  expect(onKeyDown).not.toHaveBeenCalled();
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.getByRole("textbox")).toBe(query);
  expect((query as HTMLInputElement).value).toBe("my query");
  expect(screen.getByRole("dialog")).toBe(dialog);
});
