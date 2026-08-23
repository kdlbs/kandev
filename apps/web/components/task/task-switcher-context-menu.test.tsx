import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import { TaskItemWithContextMenu } from "./task-switcher-context-menu";
import type { TaskSwitcherItem } from "./task-switcher-types";

const PLUGIN_ID = "example-task-actions";
const PLUGIN_ACTION_LABEL = "Inspect task";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

function task(overrides: Partial<TaskSwitcherItem> = {}): TaskSwitcherItem {
  return { id: "task-1", title: "Task 1", state: "IN_PROGRESS", ...overrides };
}

function ArchiveAwareRow({ archiveConfirmation }: { archiveConfirmation?: ReactNode }) {
  return <div data-testid="task-row">Task 1{archiveConfirmation}</div>;
}

/**
 * Stands in for the dnd-kit drag handle that wraps the row. In the real tree
 * the handle's sensor listeners (`onMouseDown`, `onTouchStart`, `onPointerDown`)
 * are fiber ancestors of the context-menu portal, so a pointer-start event on a
 * menu item fiber-bubbles into them and starts a row drag — unless the menu
 * stops it. `onClick` stands in for the row's `onSelectTask` click handler.
 */
function renderWithDragHandle(overrides: Partial<TaskSwitcherItem> = {}) {
  const onMouseDown = vi.fn();
  const onPointerDown = vi.fn();
  const onTouchStart = vi.fn();
  const onClick = vi.fn();
  const onArchiveTask = vi.fn();
  render(
    <StateProvider>
      <ToastProvider>
        <div
          data-testid="drag-handle"
          onMouseDown={onMouseDown}
          onPointerDown={onPointerDown}
          onTouchStart={onTouchStart}
          onClick={onClick}
        >
          <TaskItemWithContextMenu task={task(overrides)} onArchiveTask={onArchiveTask}>
            <ArchiveAwareRow />
          </TaskItemWithContextMenu>
        </div>
      </ToastProvider>
    </StateProvider>,
  );
  return { onMouseDown, onPointerDown, onTouchStart, onClick, onArchiveTask };
}

async function openContextMenu() {
  fireEvent.contextMenu(screen.getByTestId("task-row"));
  await screen.findByRole("menuitem", { name: /color/i });
}

function renderPluginMenu(run: (context: PluginTaskMenuContext) => void = vi.fn()) {
  pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
    id: "inspect-task",
    label: PLUGIN_ACTION_LABEL,
    group: "primary",
    run,
  });
  render(
    <StateProvider initialState={{ workspaces: { items: [], activeId: "workspace-1" } }}>
      <ToastProvider>
        <TaskItemWithContextMenu task={task({ workflowStepId: "step-1" })}>
          <div data-testid="plugin-task-row">Task 1</div>
        </TaskItemWithContextMenu>
      </ToastProvider>
    </StateProvider>,
  );
  fireEvent.contextMenu(screen.getByTestId("plugin-task-row"));
}

describe("TaskItemWithContextMenu — plugin primary actions", () => {
  it("runs a generic primary action with the sidebar task context", async () => {
    const run = vi.fn();
    renderPluginMenu(run);

    fireEvent.click(screen.getByRole("menuitem", { name: PLUGIN_ACTION_LABEL }));
    await Promise.resolve();

    expect(run).toHaveBeenCalledWith({
      workspaceId: "workspace-1",
      taskId: "task-1",
      taskTitle: "Task 1",
      workflowStepId: "step-1",
      presentation: "desktop",
    });
  });

  it("removes an action from an already-open menu when its plugin unregisters", async () => {
    renderPluginMenu();
    expect(screen.getByRole("menuitem", { name: PLUGIN_ACTION_LABEL })).not.toBeNull();

    pluginRegistry.unregisterPlugin(PLUGIN_ID);

    await waitFor(() => {
      expect(screen.queryByRole("menuitem", { name: PLUGIN_ACTION_LABEL })).toBeNull();
    });
  });
});

// happy-dom's TouchEvent drops the touches/changedTouches init, so the touch
// events in the cancellation tests stub it faithfully.
function stubTouchEvent() {
  vi.stubGlobal(
    "TouchEvent",
    class StubTouchEvent extends Event {
      touches: unknown[];
      changedTouches: unknown[];
      constructor(
        type: string,
        init?: EventInit & { touches?: unknown[]; changedTouches?: unknown[] },
      ) {
        // Forward bubbles/cancelable so the stub faithfully models the
        // production `new TouchEvent("touchcancel", { bubbles: true,
        // cancelable: true })` dispatch.
        super(type, init);
        this.touches = init?.touches ?? [];
        this.changedTouches = init?.changedTouches ?? [];
      }
    },
  );
}

describe("TaskItemWithContextMenu — pointer containment", () => {
  // Regression: the menu renders in a portal whose fiber ancestors include the
  // drag handle. Without a guard, mousedown/pointerdown on any menu item
  // bubbles through the React fiber tree to the handle's dnd-kit sensor
  // listeners and starts a row drag (MouseSensor ignores only right-click).
  it("mousedown and pointerdown on the Color submenu trigger do not reach the drag handle", async () => {
    const { onMouseDown, onPointerDown } = renderWithDragHandle();
    await openContextMenu();

    fireEvent.mouseDown(screen.getByRole("menuitem", { name: /color/i }));
    fireEvent.pointerDown(screen.getByRole("menuitem", { name: /color/i }));

    expect(onMouseDown).not.toHaveBeenCalled();
    expect(onPointerDown).not.toHaveBeenCalled();
  });

  it("touchstart on the Color submenu trigger and swatch does not reach the drag handle", async () => {
    const { onTouchStart } = renderWithDragHandle();
    await openContextMenu();

    fireEvent.touchStart(screen.getByRole("menuitem", { name: /color/i }));
    expect(onTouchStart).not.toHaveBeenCalled();

    fireEvent.pointerMove(screen.getByRole("menuitem", { name: /color/i }), {
      pointerType: "mouse",
    });
    const redSwatch = await screen.findByRole("menuitem", { name: /red/i });
    fireEvent.touchStart(redSwatch);
    expect(onTouchStart).not.toHaveBeenCalled();
  });

  it("mousedown and pointerdown on a color swatch do not reach the drag handle", async () => {
    const { onMouseDown, onPointerDown } = renderWithDragHandle();
    await openContextMenu();

    fireEvent.pointerMove(screen.getByRole("menuitem", { name: /color/i }), {
      pointerType: "mouse",
    });
    const redSwatch = await screen.findByRole("menuitem", { name: /red/i });
    fireEvent.mouseDown(redSwatch);
    fireEvent.pointerDown(redSwatch);

    expect(onMouseDown).not.toHaveBeenCalled();
    expect(onPointerDown).not.toHaveBeenCalled();
  });

  it("clicking a menu item runs its action without activating the row", async () => {
    const { onClick, onArchiveTask } = renderWithDragHandle();
    await openContextMenu();

    fireEvent.click(screen.getByRole("menuitem", { name: /archive/i }));

    expect(onArchiveTask).not.toHaveBeenCalled();
    const dialog = await screen.findByRole("alertdialog");
    fireEvent.click(within(dialog).getByRole("button", { name: /^Archive$/ }));
    await waitFor(() => {
      expect(onArchiveTask).toHaveBeenCalledTimes(1);
      expect(onArchiveTask).toHaveBeenCalledWith("task-1", { cascade: false });
    });
    expect(onClick).not.toHaveBeenCalled();
  });
});

describe("TaskItemWithContextMenu — touch drag cancellation", () => {
  it("opening the menu cancels an in-flight touch drag at the touchstart target", async () => {
    // A touch long-press arms the row's TouchSensor (250ms) before the menu
    // opens (~700ms); dnd-kit cancels the drag when a touchcancel reaches the
    // element the touch began on, so opening the menu must emit one there.
    stubTouchEvent();
    const touchCancelListener = vi.fn();

    try {
      renderWithDragHandle();
      const row = screen.getByTestId("task-row");
      row.addEventListener("touchcancel", touchCancelListener);

      fireEvent.touchStart(row, { touches: [{ identifier: 1 }] });
      fireEvent.contextMenu(row);
      await screen.findByRole("menuitem", { name: /color/i });

      expect(touchCancelListener).toHaveBeenCalledTimes(1);
      expect(touchCancelListener.mock.calls[0]?.[0].type).toBe("touchcancel");
      expect(touchCancelListener.mock.calls[0]?.[0].target).toBe(row);
      row.removeEventListener("touchcancel", touchCancelListener);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("a second concurrent touch does not overwrite the captured drag target", async () => {
    // dnd-kit's TouchSensor ignores a second finger (touches.length > 1), so
    // the first touch's target must stay the cancel target. The listener sits
    // on the row and `secondFinger` nests inside it, so a buggy overwrite
    // would still bubble here — assert the event target itself.
    stubTouchEvent();

    try {
      renderWithDragHandle();
      const row = screen.getByTestId("task-row");
      const secondFinger = document.createElement("div");
      row.appendChild(secondFinger);
      const touchCancelListener = vi.fn();
      row.addEventListener("touchcancel", touchCancelListener);

      fireEvent.touchStart(row, { touches: [{ identifier: 1 }] });
      fireEvent.touchStart(secondFinger, {
        touches: [{ identifier: 1 }, { identifier: 2 }],
      });
      fireEvent.contextMenu(row);
      await screen.findByRole("menuitem", { name: /color/i });

      // The cancel went to the first touch's target (the row), not the second.
      expect(touchCancelListener).toHaveBeenCalledTimes(1);
      expect(touchCancelListener.mock.calls[0]?.[0].target).toBe(row);
      row.removeEventListener("touchcancel", touchCancelListener);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("a completed touch leaves no stale cancel target for a later menu open", async () => {
    // Stub TouchEvent so the touchstart genuinely stores a target (happy-dom
    // drops the touches init); without it this assertion would pass vacuously.
    stubTouchEvent();

    try {
      renderWithDragHandle();
      const row = screen.getByTestId("task-row");
      const touchCancelListener = vi.fn();
      row.addEventListener("touchcancel", touchCancelListener);

      fireEvent.touchStart(row, { touches: [{ identifier: 1 }] });
      fireEvent.touchEnd(row, { changedTouches: [{ identifier: 1 }] });
      fireEvent.contextMenu(row);
      await screen.findByRole("menuitem", { name: /color/i });

      // No gesture is active when the menu opens, so nothing is dispatched.
      expect(touchCancelListener).not.toHaveBeenCalled();
      row.removeEventListener("touchcancel", touchCancelListener);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("a non-primary finger lifting does not clear the tracked drag target", async () => {
    // Finger 1 holds the row (long-press in progress); finger 2 lifts first.
    // Only finger 1's touchend may clear the target, so the menu-open cancel
    // still reaches the first touch's element.
    stubTouchEvent();

    try {
      renderWithDragHandle();
      const row = screen.getByTestId("task-row");
      const touchCancelListener = vi.fn();
      row.addEventListener("touchcancel", touchCancelListener);

      fireEvent.touchStart(row, { touches: [{ identifier: 1 }] });
      fireEvent.touchEnd(row, { changedTouches: [{ identifier: 2 }] });
      fireEvent.contextMenu(row);
      await screen.findByRole("menuitem", { name: /color/i });

      expect(touchCancelListener).toHaveBeenCalledTimes(1);
      expect(touchCancelListener.mock.calls[0]?.[0].type).toBe("touchcancel");
      expect(touchCancelListener.mock.calls[0]?.[0].target).toBe(row);
      row.removeEventListener("touchcancel", touchCancelListener);
    } finally {
      vi.unstubAllGlobals();
    }
  });
});

describe("TaskItemWithContextMenu — touch drag cancellation identifier lifecycle", () => {
  it("a non-primary touchcancel leaves the tracked drag target live", async () => {
    // Same rule on the touchcancel path: finger 2's cancel must not clear
    // finger 1's tracked target.
    stubTouchEvent();

    try {
      renderWithDragHandle();
      const row = screen.getByTestId("task-row");
      const touchCancelListener = vi.fn();
      row.addEventListener("touchcancel", touchCancelListener);

      fireEvent.touchStart(row, { touches: [{ identifier: 1 }] });
      fireEvent.touchCancel(row, { changedTouches: [{ identifier: 2 }] });
      // The fired touchcancel above is caught by the row listener; count only
      // what the menu-open path dispatches.
      touchCancelListener.mockClear();
      fireEvent.contextMenu(row);
      await screen.findByRole("menuitem", { name: /color/i });

      expect(touchCancelListener).toHaveBeenCalledTimes(1);
      expect(touchCancelListener.mock.calls[0]?.[0].target).toBe(row);
      row.removeEventListener("touchcancel", touchCancelListener);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("the tracked touch's touchcancel clears the cancel target", async () => {
    // When finger 1 itself is cancelled, no synthetic cancel may fire later.
    stubTouchEvent();

    try {
      renderWithDragHandle();
      const row = screen.getByTestId("task-row");
      const touchCancelListener = vi.fn();
      row.addEventListener("touchcancel", touchCancelListener);

      fireEvent.touchStart(row, { touches: [{ identifier: 1 }] });
      fireEvent.touchCancel(row, { changedTouches: [{ identifier: 1 }] });
      touchCancelListener.mockClear();
      fireEvent.contextMenu(row);
      await screen.findByRole("menuitem", { name: /color/i });

      expect(touchCancelListener).not.toHaveBeenCalled();
      row.removeEventListener("touchcancel", touchCancelListener);
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
