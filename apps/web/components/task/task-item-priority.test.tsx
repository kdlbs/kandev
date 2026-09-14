import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { TaskItem } from "./task-item";

afterEach(cleanup);

function renderPriorityItem(priority: "high" | "medium") {
  return render(
    <StateProvider>
      <TooltipProvider>
        <TaskItem
          title="Priority task"
          priority={priority}
          prInfo={{ number: 42, state: "Open" }}
        />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("TaskItem priority", () => {
  it("renders priority after the title and before a linked PR badge", () => {
    renderPriorityItem("high");

    const title = screen.getByText("Priority task");
    const priority = screen.getByTestId("sidebar-task-priority-indicator");
    const prIcon = screen.getByTestId("pr-task-icon");
    expect(title.compareDocumentPosition(priority) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(
      priority.compareDocumentPosition(prIcon) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("keeps the default medium priority visually quiet", () => {
    renderPriorityItem("medium");

    expect(screen.queryByTestId("sidebar-task-priority-indicator")).toBeNull();
  });
});
