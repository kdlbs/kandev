import type { ReactNode } from "react";
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TaskSwitcherItem } from "./task-switcher-types";

const taskItemRender = vi.hoisted(() => vi.fn());

vi.mock("./task-item", () => ({
  TaskItem: ({ title }: { title: string }) => {
    taskItemRender();
    return <div>{title}</div>;
  },
}));

vi.mock("./task-switcher-context-menu", () => ({
  TaskItemWithContextMenu: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

import { TaskRow, type TaskRowProps } from "./task-switcher-row";

const TASK: TaskSwitcherItem = { id: "task-1", title: "Task 1", state: "IN_PROGRESS" };

afterEach(() => {
  cleanup();
  taskItemRender.mockReset();
});

describe("TaskRow render isolation", () => {
  it("skips an unrelated owner render when the row props remain equal", () => {
    const stableProps: TaskRowProps = {
      task: TASK,
      activeTaskId: null,
      selectedTaskId: null,
      onSelectTask: vi.fn(),
    };
    const { rerender } = render(<TaskRow {...stableProps} />);

    rerender(<TaskRow {...stableProps} />);

    expect(taskItemRender).toHaveBeenCalledTimes(1);
  });
});
