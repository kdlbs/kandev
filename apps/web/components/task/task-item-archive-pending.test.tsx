import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { TaskItem } from "./task-item";

afterEach(() => cleanup());

describe("TaskItem pending archive state", () => {
  it("dims the row and shows a spinner while archive is pending", () => {
    render(
      <StateProvider>
        <TooltipProvider>
          <TaskItem title="Needs answer" state="REVIEW" isPendingArchive />
        </TooltipProvider>
      </StateProvider>,
    );

    const row = screen.getByTestId("sidebar-task-item");
    expect(row.getAttribute("aria-busy")).toBe("true");
    expect(row.getAttribute("aria-disabled")).toBe("true");
    expect(row.className).toContain("opacity-60");
    expect(screen.getByTestId("task-state-archive-pending").className).toContain("animate-spin");
    expect(screen.queryByTestId("task-state-turn-finished")).toBeNull();
  });
});
