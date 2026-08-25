import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TaskItemTrailing } from "./task-item-trailing";

afterEach(cleanup);

describe("TaskItemTrailing relative time", () => {
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
});
