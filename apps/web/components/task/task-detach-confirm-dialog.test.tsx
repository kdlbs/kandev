import { useRef, useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  TaskDetachConfirmPopover,
  TaskDetachInlineConfirmation,
  TaskDetachTargetConfirmDialog,
  TaskDetachConfirmationSurface,
} from "./task-detach-confirm-dialog";

afterEach(() => {
  cleanup();
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 1024 });
});

it("routes phone detach to a sheet with the named hierarchy-only consequence", async () => {
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
  const onConfirm = vi.fn();
  function Phone() {
    const [open, setOpen] = useState(true);
    const anchorRef = useRef<HTMLButtonElement>(null);
    return (
      <>
        <button ref={anchorRef}>Child task</button>
        <TaskDetachConfirmationSurface
          taskId="child"
          open={open}
          anchorRef={anchorRef}
          taskTitle="Child task"
          sharesParentWorkspace
          onOpenChange={setOpen}
          onConfirm={onConfirm}
        />
      </>
    );
  }
  render(<Phone />);
  const sheet = await screen.findByRole("dialog", { name: "Detach task from parent?" });
  expect(sheet.getAttribute("data-slot")).toBe("drawer-content");
  expect(sheet.textContent).toContain("Child task");
  expect(sheet.textContent).toContain("shares its parent's workspace");
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(onConfirm).not.toHaveBeenCalled();
});

function PopoverHarness({ onConfirm = vi.fn() }: { onConfirm?: () => void | Promise<void> }) {
  const [open, setOpen] = useState(true);
  const anchorRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      <button ref={anchorRef} type="button" data-testid="detach-anchor">
        Child task
      </button>
      <TaskDetachConfirmPopover
        open={open}
        anchorRef={anchorRef}
        focusReturnRef={anchorRef}
        restoreFocusOnConfirm
        taskTitle="Child task"
        sharesParentWorkspace
        onOpenChange={setOpen}
        onConfirm={onConfirm}
      />
    </>
  );
}

describe("task detach confirmation adapters", () => {
  it("uses the local popover while preserving hierarchy-only copy", async () => {
    render(<PopoverHarness />);

    const confirmation = await screen.findByRole("dialog", { name: "Detach task from parent?" });
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(confirmation.textContent).toContain("workflow, subtasks, and state will not change");
    expect(confirmation.textContent).toContain("shares its parent's workspace");
    expect(screen.getByTestId("detach-task-confirm").classList.contains("min-h-11")).toBe(true);
  });

  it("keeps inline touch actions local and closes before confirming", async () => {
    let shellClosed = false;
    const onConfirm = vi.fn(() => {
      shellClosed = screen.queryByTestId("detach-task-inline-confirmation") === null;
    });
    function Harness() {
      const [open, setOpen] = useState(true);
      if (!open) return null;
      return (
        <TaskDetachInlineConfirmation
          taskTitle="Child task"
          onCancel={() => setOpen(false)}
          onClose={() => setOpen(false)}
          onConfirm={onConfirm}
        />
      );
    }

    render(<Harness />);
    const confirm = screen.getByTestId("detach-task-confirm");
    expect(confirm.classList.contains("h-11")).toBe(true);
    fireEvent.click(confirm);

    await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce());
    expect(shellClosed).toBe(true);
    expect(screen.queryByTestId("detach-task-inline-confirmation")).toBeNull();
  });

  it("returns focus to the menu trigger after confirmation", async () => {
    const onConfirm = vi.fn();
    render(<PopoverHarness onConfirm={onConfirm} />);

    fireEvent.click(screen.getByTestId("detach-task-confirm"));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce());
    await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId("detach-anchor")));
  });

  it("keeps task-switcher detachment on its existing modal branch", async () => {
    const longTaskTitle = `Child task ${"x".repeat(180)}`;
    render(
      <TaskDetachTargetConfirmDialog
        target={{ id: "child", title: longTaskTitle, workspaceMode: "inherit_parent" }}
        detachingTaskId={null}
        onDismiss={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    await waitFor(() => {
      const dialog = screen.getByRole("alertdialog", { name: "Detach task from parent?" });
      const description = dialog.querySelector('[data-slot="alert-dialog-description"]');
      expect(description).not.toBeNull();
      expect(description?.id).toBe(dialog.getAttribute("aria-describedby"));
      expect(description?.classList.contains("min-w-0")).toBe(true);
      expect(description?.classList.contains("text-left")).toBe(true);
      expect(description?.querySelectorAll("p")).toHaveLength(3);
      expect(description?.textContent).toContain(longTaskTitle);
    });
  });
});
