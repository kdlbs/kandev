import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TaskSwitcherItem } from "./task-switcher-types";

const view = {
  id: "view-1",
  name: "By workflow",
  filters: [],
  sort: { key: "title" as const, direction: "asc" as const },
  group: "workflow" as const,
  collapsedGroups: [],
};

const prefs = {
  pinnedTaskIds: [],
  orderedTaskIds: [],
  subtaskOrderByParentId: {},
  togglePinnedTask: vi.fn(),
  handleReorderGroup: vi.fn(),
  handleReorderSubtasks: vi.fn(),
};

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ i18n: { language: "en" } }),
}));

vi.mock("@/hooks/domains/sidebar/use-effective-sidebar-view", () => ({
  useEffectiveSidebarView: () => view,
}));

vi.mock("@/hooks/domains/sidebar/use-sidebar-task-prefs", () => ({
  useSidebarTaskPrefs: () => prefs,
}));

import { useGroupedSidebarView } from "./task-session-sidebar-grouped-view";

function task(id: string, workflowId: string): TaskSwitcherItem {
  return { id, title: id, state: "IN_PROGRESS", workflowId, workflowName: workflowId };
}

describe("useGroupedSidebarView", () => {
  it("preserves an unaffected group reference when a task in another group updates", () => {
    const taskA = task("Task A", "Workflow A");
    const taskB = task("Task B", "Workflow B");
    const hook = renderHook(({ tasks }) => useGroupedSidebarView(tasks), {
      initialProps: { tasks: [taskA, taskB] },
    });
    const unaffectedGroup = hook.result.current.grouped.groups.find(
      (group) => group.key === "Workflow B",
    );

    hook.rerender({ tasks: [{ ...taskA, title: "Task A updated" }, { ...taskB }] });

    expect(hook.result.current.grouped.groups[1]).toBe(unaffectedGroup);
  });
});
