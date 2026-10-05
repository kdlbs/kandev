import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { GroupHeader } from "./task-switcher-group";

afterEach(cleanup);

it("announces collapse and preserves the continued-page count", () => {
  const toggle = vi.fn();
  render(
    <GroupHeader
      label="Review"
      groupKey="REVIEW"
      grouping="state"
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

it("uses completion semantics only for the completed state group", () => {
  render(
    <GroupHeader
      label="Completed"
      groupKey="COMPLETED"
      grouping="state"
      count={1}
      isCollapsed={false}
      onToggle={() => {}}
    />,
  );
  expect(screen.getByTestId("task-state-workflow-complete")).toBeTruthy();
});

it.each([
  ["BLOCKED", "tabler-icon-alert-circle", "text-yellow-500"],
  ["FAILED", "tabler-icon-x", "text-red-500"],
  ["CANCELLED", "tabler-icon-x", "text-red-500"],
])("keeps the %s group distinguishable from backlog", (state, icon, color) => {
  render(
    <GroupHeader
      label={state}
      groupKey={state}
      grouping="state"
      count={1}
      isCollapsed={false}
      onToggle={() => {}}
    />,
  );
  const indicator = screen.getByTestId("sidebar-group-state");
  expect(indicator.querySelector(`.${icon}.${color}`)).not.toBeNull();
  expect(screen.queryByTestId("task-state-backlog")).toBeNull();
});

it.each(["repository", "workflow", undefined] as const)(
  "does not mistake a %s name for state",
  (grouping) => {
    render(
      <GroupHeader
        label="Completed"
        groupKey="COMPLETED"
        grouping={grouping}
        count={1}
        isCollapsed={false}
        onToggle={() => {}}
      />,
    );
    expect(screen.queryByTestId("sidebar-group-state")).toBeNull();
  },
);

it("keeps unknown states neutral", () => {
  render(
    <GroupHeader
      label="Future"
      groupKey="future-state"
      grouping="state"
      count={1}
      isCollapsed={false}
      onToggle={() => {}}
    />,
  );
  expect(screen.queryByTestId("sidebar-group-state")).toBeNull();
});
