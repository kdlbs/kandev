import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskSwitcher, type TaskSwitcherItem } from "./task-switcher";
import { GroupHeader } from "./task-switcher-group";

afterEach(cleanup);

it("announces collapse and preserves the continued-page count", () => {
  const toggle = vi.fn();
  render(
    <GroupHeader
      label="Review"
      groupKey="REVIEW"
      count={125}
      isCollapsed
      isContinuation
      onToggle={toggle}
    />,
  );
  const button = screen.getByRole("button");
  expect(button.getAttribute("aria-expanded")).toBe("false");
  expect(button.textContent).toContain("125");
  fireEvent.click(button);
  expect(toggle).toHaveBeenCalledOnce();
});

it.each([
  "NOT_STARTED",
  "IN_PROGRESS",
  "REVIEW",
  "COMPLETED",
  "BLOCKED",
  "FAILED",
  "CANCELLED",
  "future-state",
  "repository-name",
])("renders the %s header with only the collapse icon", (groupKey) => {
  render(
    <GroupHeader
      label={groupKey}
      groupKey={groupKey}
      count={3}
      isCollapsed={false}
      onToggle={() => {}}
    />,
  );
  const button = screen.getByRole("button");
  expect(button.getAttribute("aria-expanded")).toBe("true");
  expect(button.textContent).toContain(groupKey);
  expect(button.textContent).toContain("3");
  expect(button.querySelectorAll("svg")).toHaveLength(1);
  expect(button.querySelector(".tabler-icon-chevron-down")).not.toBeNull();
  expect(screen.queryByTestId("sidebar-group-state")).toBeNull();
});

it("keeps group inset configurable without changing descendant depth", () => {
  const root: TaskSwitcherItem = { id: "Root", title: "Root" };
  const child: TaskSwitcherItem = { id: "Child", title: "Child", parentTaskId: "Root" };
  const grandchild: TaskSwitcherItem = {
    id: "Grandchild",
    title: "Grandchild",
    parentTaskId: "Child",
  };
  const renderGrouped = (groupIndent?: boolean) =>
    render(
      <StateProvider>
        <ToastProvider>
          <TaskSwitcher
            grouped={{
              groups: [{ key: "workflow-1", label: "Workflow 1", tasks: [root] }],
              subTasksByParentId: new Map([
                ["Root", [child]],
                ["Child", [grandchild]],
              ]),
            }}
            groupIndent={groupIndent}
            activeTaskId={null}
            selectedTaskId={null}
            onSelectTask={vi.fn()}
          />
        </ToastProvider>
      </StateProvider>,
    );
  const enabled = renderGrouped();
  const body = enabled.container.querySelector('[data-testid="sidebar-group"] [role="group"]');
  expect(body?.classList.contains("ml-5")).toBe(true);
  expect(blockDepth(enabled.container, "Grandchild")).toBe("2");
  enabled.unmount();

  const disabled = renderGrouped(false);
  const unindentedBody = disabled.container.querySelector(
    '[data-testid="sidebar-group"] [role="group"]',
  );
  expect(unindentedBody?.classList.contains("ml-5")).toBe(false);
  expect(screen.getByTestId("sidebar-group-header")).toBeTruthy();
  expect(blockDepth(disabled.container, "Grandchild")).toBe("2");
});

function blockDepth(container: HTMLElement, taskId: string): string | null {
  return container.querySelector(`[data-task-id="${taskId}"]`)?.getAttribute("data-depth") ?? null;
}
