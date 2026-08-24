import { useRef, useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  TaskDetachConfirmPopover,
  TaskDetachInlineConfirmation,
  TaskDetachTargetConfirmDialog,
} from "./task-detach-confirm-dialog";

afterEach(cleanup);

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

  it("keeps task-switcher detachment on its existing modal branch", async () => {
    render(
      <TaskDetachTargetConfirmDialog
        target={{ id: "child", title: "Child task", workspaceMode: "inherit_parent" }}
        detachingTaskId={null}
        onDismiss={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    await waitFor(() =>
      expect(screen.getByRole("alertdialog", { name: "Detach task from parent?" })).not.toBeNull(),
    );
  });
});
