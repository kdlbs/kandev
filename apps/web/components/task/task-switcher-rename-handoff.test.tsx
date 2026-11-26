import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskFlagEditorProvider } from "./task-flag-editor-provider";
import { TaskItemWithContextMenu } from "./task-switcher-context-menu";

afterEach(cleanup);

it("opens rename only after the context menu releases focus", async () => {
  const menuPresentAtRename: boolean[] = [];
  const onRenameTask = vi.fn(() => {
    menuPresentAtRename.push(screen.queryByRole("menu") !== null);
  });
  render(
    <StateProvider>
      <ToastProvider>
        <TaskItemWithContextMenu
          task={{ id: "task-1", title: "Task 1", state: "IN_PROGRESS" }}
          onRenameTask={onRenameTask}
        >
          <div data-testid="task-row">Task 1</div>
        </TaskItemWithContextMenu>
      </ToastProvider>
    </StateProvider>,
  );
  fireEvent.contextMenu(screen.getByTestId("task-row"));
  await screen.findByRole("menuitem", { name: "Rename" });
  fireEvent.click(screen.getByRole("menuitem", { name: "Rename" }));
  await waitFor(() => expect(onRenameTask).toHaveBeenCalledExactlyOnceWith("task-1", "Task 1"));
  expect(menuPresentAtRename).toEqual([false]);
});

it("hands off from Add flag to the editor and restores row focus on cancel", async () => {
  render(
    <StateProvider>
      <ToastProvider>
        <TaskFlagEditorProvider>
          <TaskItemWithContextMenu
            task={{ id: "other-task", title: "Other task", state: "IN_PROGRESS" }}
          >
            <div data-testid="task-row">Other task</div>
          </TaskItemWithContextMenu>
        </TaskFlagEditorProvider>
      </ToastProvider>
    </StateProvider>,
  );
  fireEvent.contextMenu(screen.getByTestId("task-row"));
  fireEvent.click(await screen.findByRole("menuitem", { name: "Add flag" }));
  await screen.findByRole("dialog", { name: "Add flag" });
  expect(screen.queryByRole("menu")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  await waitFor(() =>
    expect(document.activeElement?.getAttribute("data-slot")).toBe("context-menu-trigger"),
  );
});
