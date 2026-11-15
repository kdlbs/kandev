import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, within } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskSwitcher, type TaskSwitcherItem } from "./task-switcher";

afterEach(() => cleanup());

const UNREAD_LABEL = "Task has unread messages";

function item(id: string, overrides: Partial<TaskSwitcherItem> = {}): TaskSwitcherItem {
  return { id, title: id, state: "IN_PROGRESS", ...overrides };
}

const UNREAD_A = item("Task A", { unreadSessionIds: ["a-1"] });
const UNREAD_B = item("Task B", { unreadSessionIds: ["b-1"] });
const READ_C = item("Task C");

function switcher(
  enabled: boolean,
  activeTaskId: string | null = null,
  hideDelayMs = 0,
  displayedSessionId: string | null = null,
  tasks: TaskSwitcherItem[] = [UNREAD_A, UNREAD_B, READ_C],
) {
  return (
    <StateProvider>
      <ToastProvider>
        <TaskSwitcher
          grouped={{
            groups: [{ key: "__all__", label: "All", tasks }],
            subTasksByParentId: new Map(),
          }}
          activeTaskId={activeTaskId}
          selectedTaskId={null}
          unreadMessagesEnabled={enabled}
          unreadDotHideDelayMs={hideDelayMs}
          displayedSessionId={displayedSessionId}
          onSelectTask={vi.fn()}
        />
      </ToastProvider>
    </StateProvider>
  );
}

function rowFor(taskId: string): HTMLElement {
  const row = document.querySelector<HTMLElement>(`[data-task-row-id="${taskId}"]`);
  if (!row) throw new Error(`row ${taskId} is not rendered`);
  return row;
}

describe("TaskSwitcher unread dot", () => {
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.1
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.5
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.13
  it("labels unread rows other than the open task", () => {
    render(switcher(true, UNREAD_B.id, 0, "b-1"));

    expect(within(rowFor(UNREAD_A.id)).getByRole("img", { name: UNREAD_LABEL })).not.toBeNull();
    expect(within(rowFor(UNREAD_B.id)).queryByRole("img", { name: UNREAD_LABEL })).toBeNull();
    expect(within(rowFor(READ_C.id)).queryByRole("img", { name: UNREAD_LABEL })).toBeNull();
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.17
  it("keeps the open task's dot while another of its sessions is unread", () => {
    const multi = item("Task D", { unreadSessionIds: ["d-1", "d-2"] });
    render(switcher(true, multi.id, 0, "d-1", [multi, READ_C]));

    expect(within(rowFor(multi.id)).getByRole("img", { name: UNREAD_LABEL })).not.toBeNull();
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.3
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.4
  it("follows the Unread Messages setting", () => {
    const view = render(switcher(false));
    expect(screen.queryAllByRole("img", { name: UNREAD_LABEL })).toHaveLength(0);

    view.rerender(switcher(true));
    expect(screen.queryAllByRole("img", { name: UNREAD_LABEL })).toHaveLength(2);
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.5
  it("keeps the opened task's dot for the hide delay, then hides it", () => {
    vi.useFakeTimers();
    try {
      const view = render(switcher(true, null, 5000));
      view.rerender(switcher(true, UNREAD_A.id, 5000, "a-1"));
      expect(within(rowFor(UNREAD_A.id)).getByRole("img", { name: UNREAD_LABEL })).not.toBeNull();

      act(() => vi.advanceTimersByTime(5000));
      expect(within(rowFor(UNREAD_A.id)).queryByRole("img", { name: UNREAD_LABEL })).toBeNull();
      expect(within(rowFor(UNREAD_B.id)).getByRole("img", { name: UNREAD_LABEL })).not.toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });
});
