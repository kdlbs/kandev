import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskItemWithContextMenu } from "./task-switcher-context-menu";
import type { TaskSwitcherItem } from "./task-switcher-types";

afterEach(cleanup);

function task(overrides: Partial<TaskSwitcherItem> = {}): TaskSwitcherItem {
  return { id: "task-1", title: "Task 1", state: "IN_PROGRESS", ...overrides };
}

function ArchiveAwareRow() {
  return <div data-testid="task-row">Task 1</div>;
}

describe("TaskItemWithContextMenu — single-selection bulk archive", () => {
  it("keeps Archive for a one-row selection with only the bulk archive handler", async () => {
    const onBulkArchive = vi.fn();
    render(
      <StateProvider>
        <ToastProvider>
          <TaskItemWithContextMenu
            task={task()}
            selectedTaskIds={new Set(["task-1"])}
            onBulkArchive={onBulkArchive}
          >
            <ArchiveAwareRow />
          </TaskItemWithContextMenu>
        </ToastProvider>
      </StateProvider>,
    );

    fireEvent.contextMenu(screen.getByTestId("task-row"));
    const archive = await screen.findByRole("menuitem", { name: "Archive" });

    fireEvent.click(archive);

    expect(onBulkArchive).toHaveBeenCalledWith(["task-1"]);
  });

  it("does not show a removal group for a non-selected row with only the bulk archive handler", async () => {
    render(
      <StateProvider>
        <ToastProvider>
          <TaskItemWithContextMenu
            task={task()}
            selectedTaskIds={new Set(["task-2"])}
            onBulkArchive={vi.fn()}
          >
            <ArchiveAwareRow />
          </TaskItemWithContextMenu>
        </ToastProvider>
      </StateProvider>,
    );

    fireEvent.contextMenu(screen.getByTestId("task-row"));
    const menu = await screen.findByRole("menu");

    expect(within(menu).queryByRole("menuitem", { name: /archive/i })).toBeNull();
  });
});
