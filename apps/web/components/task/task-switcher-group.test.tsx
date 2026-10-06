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
