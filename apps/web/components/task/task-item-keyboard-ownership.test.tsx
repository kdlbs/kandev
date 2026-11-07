import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { TaskItem } from "./task-item";

afterEach(cleanup);

function renderRow(props: Partial<React.ComponentProps<typeof TaskItem>> = {}) {
  const onSelect = vi.fn();
  const onContextMenu = vi.fn();
  render(
    <StateProvider>
      <TooltipProvider>
        <div onContextMenu={onContextMenu}>
          <TaskItem title="Keyboard target" state="IN_PROGRESS" onSelect={onSelect} {...props} />
        </div>
      </TooltipProvider>
    </StateProvider>,
  );
  return { onSelect, onContextMenu, row: screen.getByTestId("sidebar-task-item") };
}

describe("sidebar task row keyboard ownership", () => {
  // @covers AC-TASKS-KEYBOARD-ACTIONS-003.1
  it.each(["Enter", " "])("leaves %s on the real task action button to the button", (key) => {
    const { onSelect } = renderRow();
    const button = screen.getByRole("button", { name: "Task actions" });
    const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true });
    button.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("leaves keydowns from the button's icon to the button", () => {
    const { onSelect } = renderRow();
    const icon = screen.getByRole("button", { name: "Task actions" }).querySelector("svg");
    expect(icon).not.toBeNull();
    const event = new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true });
    icon!.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("keeps the actual button click and bubbling context menu separate from row selection", () => {
    const { onSelect, onContextMenu } = renderRow();
    fireEvent.click(screen.getByRole("button", { name: "Task actions" }));
    expect(onContextMenu).toHaveBeenCalledTimes(1);
    expect(onSelect).not.toHaveBeenCalled();
  });

  it.each(["Enter", " "])("retains row activation and modifiers for %s", (key) => {
    const { onSelect, row } = renderRow({ isSelected: true, isMultiSelected: true });
    fireEvent.keyDown(row, { key, ctrlKey: true, shiftKey: true, repeat: true });
    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onSelect.mock.calls[0][0]).toMatchObject({
      ctrlKey: true,
      shiftKey: true,
      repeat: true,
    });
  });

  it("retains title-origin and already-prevented row key behavior", () => {
    const { onSelect, row } = renderRow();
    fireEvent.keyDown(screen.getByText("Keyboard target"), { key: "Enter" });
    const event = new KeyboardEvent("keydown", { key: " ", bubbles: true, cancelable: true });
    event.preventDefault();
    row.dispatchEvent(event);
    expect(onSelect).toHaveBeenCalledTimes(2);
  });

  it("suppresses row activation while removal is pending", () => {
    const { onSelect, row } = renderRow({ isPendingRemoval: true });
    const event = new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true });
    row.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("keeps the subtask toggle from activating the row", () => {
    const toggle = vi.fn();
    const { onSelect } = renderRow({ taskId: "parent", subtaskCount: 1, onToggleSubtasks: toggle });
    const button = screen.getByRole("button", { name: /subtask/i });
    fireEvent.keyDown(button, { key: "Enter" });
    fireEvent.click(button);
    expect(toggle).toHaveBeenCalledTimes(1);
    expect(onSelect).not.toHaveBeenCalled();
  });
});
