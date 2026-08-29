import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskItemTrailing } from "./task-item-trailing";

vi.mock("./task-contribution-icons", () => ({
  TaskContributionIcons: () => null,
}));

vi.mock("@/components/integrations/registered-change-request-task-icon", () => ({
  RegisteredChangeRequestTaskIcon: () => null,
}));

afterEach(cleanup);

describe("TaskItemTrailing relative time", () => {
  it("renders a compact value with the full relative time as its accessible name", () => {
    const relativeTimeValue = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString();
    render(
      <TaskItemTrailing
        trailing="relative_time"
        menuOpen={false}
        effectiveMenuOpen={false}
        relativeTime={relativeTimeValue}
      />,
    );

    const relativeTime = screen.getByTestId("sidebar-task-trailing-time");
    expect(relativeTime.querySelector('[aria-hidden="true"]')?.textContent).toBe("2d");
    expect(relativeTime.querySelector(".sr-only")?.textContent).toBe("2 days ago");
    expect(relativeTime.getAttribute("aria-label")).toBeNull();
    expect(relativeTime.getAttribute("title")).toBe("2 days ago");
    expect(relativeTime.className).toContain("w-11");
    expect(relativeTime.className).toContain("text-right");
    expect(relativeTime.className).toContain("tabular-nums");
    expect(relativeTime.parentElement?.className).toContain("[@media(max-width:639px)]:w-auto");
  });

  it("uses the outer task-row menu disclosure hover and focus selectors", () => {
    render(
      <TaskItemTrailing
        trailing="relative_time"
        menuOpen={false}
        effectiveMenuOpen={false}
        relativeTime="2026-07-24T00:00:00Z"
      />,
    );

    const relativeTime = screen.getByTestId("sidebar-task-trailing-time");
    expect(relativeTime.className).toContain("group-hover:opacity-0");
    expect(relativeTime.className).toContain("group-focus-within/actions:opacity-0");
    expect(relativeTime.className).not.toContain("group-hover/actions:opacity-0");
  });

  it("omits an invalid timestamp instead of reserving a time column", () => {
    render(
      <TaskItemTrailing
        trailing="relative_time"
        menuOpen={false}
        effectiveMenuOpen={false}
        relativeTime="not-a-date"
      />,
    );

    expect(screen.queryByTestId("sidebar-task-trailing-time")).toBeNull();
    expect(screen.getByRole("button", { name: "Task actions" })).not.toBeNull();
  });
});

describe("TaskItemTrailing change-request status", () => {
  it("does not reserve the hidden menu width while the row is idle", () => {
    render(
      <TaskItemTrailing
        trailing="change_request_status"
        menuOpen={false}
        effectiveMenuOpen={false}
        prInfo={{ number: 42, state: "open" }}
      />,
    );

    const status = screen.getByTestId("sidebar-task-change-request-status");
    const menuSlot = screen.getByTestId("sidebar-task-change-request-menu-slot");

    expect(status).not.toBeNull();
    expect(status.className).toContain("empty:hidden");
    expect(menuSlot?.className).toContain("w-0");
    expect(menuSlot?.className).toContain("min-w-0");
    expect(menuSlot?.className).toContain("group-hover:w-6");
    expect(menuSlot?.className).toContain("group-focus-within:w-6");
  });

  it("falls back to the task menu when no change-request data exists", () => {
    render(
      <TaskItemTrailing
        trailing="change_request_status"
        menuOpen={false}
        effectiveMenuOpen={false}
      />,
    );

    expect(screen.queryByTestId("sidebar-task-change-request-status")).toBeNull();
    expect(screen.getByRole("button", { name: "Task actions" })).not.toBeNull();
  });

  it("keeps the menu-only layout when a task has no change-request status", () => {
    render(
      <TaskItemTrailing
        trailing="change_request_status"
        menuOpen={false}
        effectiveMenuOpen={false}
        taskId="task-without-change-request"
      />,
    );

    const status = screen.getByTestId("sidebar-task-change-request-status");
    const actions = screen.getByTestId("sidebar-task-change-request-actions");
    const menuSlot = screen.getByTestId("sidebar-task-change-request-menu-slot");

    expect(status.childElementCount).toBe(0);
    expect(status.className).toContain("empty:hidden");
    expect(actions.contains(screen.getByRole("button", { name: "Task actions" }))).toBe(true);
    expect(menuSlot.className).toContain("w-0");
  });
});
