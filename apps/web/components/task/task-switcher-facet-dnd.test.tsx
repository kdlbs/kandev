import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyView } from "@/lib/sidebar/apply-view";
import { groupSidebarTasksByFacet, sidebarPageTasks } from "@/lib/sidebar/sidebar-facet-groups";
import { DEFAULT_VIEW } from "@/lib/state/slices/ui/sidebar-view-builtins";
import type { TaskSwitcherItem } from "./task-switcher-types";

vi.mock("./task-item", () => ({
  TaskItem: ({ title }: { title: string }) => <span>{title}</span>,
}));
vi.mock("./task-switcher-context-menu", () => ({
  TaskItemWithContextMenu: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock("./task-switcher-subtask-dnd", () => ({
  SortableTaskLevel: ({
    tasks,
    onReorder,
    renderNode,
  }: {
    tasks: TaskSwitcherItem[];
    onReorder?: (ids: string[]) => void;
    renderNode: (task: TaskSwitcherItem, draggable: boolean) => ReactNode;
  }) => (
    <div data-testid="task-level" data-task-ids={tasks.map((task) => task.id).join(",")}>
      {onReorder && (
        <button onClick={() => onReorder(tasks.map((task) => task.id).reverse())}>
          Reorder {tasks.map((task) => task.id).join(",")}
        </button>
      )}
      {tasks.map((task) => renderNode(task, !!onReorder))}
    </div>
  ),
  SortableTaskNode: ({ handle, nested }: { handle: ReactNode; nested?: ReactNode }) => (
    <>
      {handle}
      {nested}
    </>
  ),
  TaskTreeDndGroup: ({
    children,
    onReorderGroup,
  }: {
    children: (targets: Set<string>) => ReactNode;
    onReorderGroup?: (ids: string[]) => void;
  }) => (
    <div data-testid="tree-dnd-group" data-root-reorder-enabled={!!onReorderGroup}>
      {children(new Set())}
    </div>
  ),
  NestDropZone: () => null,
}));

import { TaskSwitcher } from "./task-switcher";

const FACET = "facet:drag-test:tags";
const alpha = { value: "alpha", label: "Alpha" };
const beta = { value: "beta", label: "Beta" };
const task = (id: string, parentTaskId?: string): TaskSwitcherItem => ({
  id,
  title: id,
  parentTaskId,
});
const BETA_PARENT_ID = "beta-parent";
const tasks = [
  task("parent"),
  task("peer"),
  task("child-a", "parent"),
  task("child-b", "parent"),
  task(BETA_PARENT_ID),
  task("nested-a", BETA_PARENT_ID),
  task("nested-b", BETA_PARENT_ID),
];

function facetPage(includeParentInBeta: boolean) {
  const page = applyView(
    tasks,
    { ...DEFAULT_VIEW, group: FACET, sort: { key: "title", direction: "asc" } },
    {
      pinnedTaskIds: [],
      orderedTaskIds: [],
      subtaskOrderByParentId: {
        parent: ["child-b", "child-a"],
        [BETA_PARENT_ID]: ["nested-b", "nested-a"],
      },
    },
  );
  return groupSidebarTasksByFacet(sidebarPageTasks(page), FACET, {
    [`${FACET}:parent`]: includeParentInBeta ? [alpha, beta] : [alpha],
    [`${FACET}:peer`]: [alpha],
    [`${FACET}:child-a`]: [beta],
    [`${FACET}:child-b`]: [beta],
    [`${FACET}:${BETA_PARENT_ID}`]: [beta],
    [`${FACET}:nested-a`]: [beta],
    [`${FACET}:nested-b`]: [beta],
  });
}

function renderSwitcher(includeParentInBeta = false) {
  const onReorderGroup = vi.fn();
  const onReorderSubtasks = vi.fn();
  render(
    <TaskSwitcher
      grouped={facetPage(includeParentInBeta)}
      activeTaskId={null}
      selectedTaskId={null}
      onSelectTask={vi.fn()}
      onReorderGroup={onReorderGroup}
      onReorderSubtasks={onReorderSubtasks}
    />,
  );
  return { onReorderGroup, onReorderSubtasks };
}

function section(label: string) {
  return within(screen.getByText(label).closest<HTMLElement>("[data-testid='sidebar-group']")!);
}

afterEach(cleanup);

describe("facet section drag ordering", () => {
  it("disables promoted-root reorder with saved sibling order but keeps other roots and nested siblings draggable", () => {
    const { onReorderGroup, onReorderSubtasks } = renderSwitcher();
    const betaSection = section("Beta");
    expect(betaSection.getAllByTestId("task-level")[0].getAttribute("data-task-ids")).toBe(
      "beta-parent,child-b,child-a",
    );
    expect(
      betaSection.queryByRole("button", { name: "Reorder beta-parent,child-b,child-a" }),
    ).toBeNull();
    expect(
      betaSection.getByTestId("tree-dnd-group").getAttribute("data-root-reorder-enabled"),
    ).toBe("false");
    expect(
      section("Alpha").getByTestId("tree-dnd-group").getAttribute("data-root-reorder-enabled"),
    ).toBe("true");

    fireEvent.click(betaSection.getByRole("button", { name: "Reorder nested-b,nested-a" }));
    expect(onReorderSubtasks).toHaveBeenCalledWith(BETA_PARENT_ID, ["nested-a", "nested-b"]);
    expect(onReorderGroup).not.toHaveBeenCalled();

    fireEvent.click(section("Alpha").getByRole("button", { name: "Reorder parent,peer" }));
    expect(onReorderGroup).toHaveBeenCalledWith(["peer", "parent"]);
  });

  it("keeps real-root and ordinary sibling reorder enabled when the parent is in the same facet section", () => {
    const { onReorderGroup, onReorderSubtasks } = renderSwitcher(true);
    const betaSection = section("Beta");
    expect(
      betaSection.getByTestId("tree-dnd-group").getAttribute("data-root-reorder-enabled"),
    ).toBe("true");
    fireEvent.click(betaSection.getByRole("button", { name: "Reorder beta-parent,parent" }));
    expect(onReorderGroup).toHaveBeenCalledWith(["parent", BETA_PARENT_ID]);
    fireEvent.click(betaSection.getByRole("button", { name: "Reorder child-b,child-a" }));
    expect(onReorderSubtasks).toHaveBeenCalledWith("parent", ["child-a", "child-b"]);
  });
});
