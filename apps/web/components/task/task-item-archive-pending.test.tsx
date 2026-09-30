import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { TaskItem } from "./task-item";

afterEach(() => cleanup());

describe("TaskItem pending removal state", () => {
  it.each([false, true])(
    "dims the pending row and replaces its state icon (archived=%s)",
    (isArchived) => {
      render(
        <StateProvider>
          <TooltipProvider>
            <TaskItem
              title="Needs answer"
              state="REVIEW"
              isArchived={isArchived}
              isPendingRemoval
            />
          </TooltipProvider>
        </StateProvider>,
      );

      const row = screen.getByTestId("sidebar-task-item");
      expect(row.getAttribute("aria-busy")).toBe("true");
      expect(row.getAttribute("aria-disabled")).toBe("true");
      expect(row.className).toContain("opacity-60");
      expect(screen.getByTestId("task-state-removal-pending").className).toContain("animate-spin");
      expect(screen.queryByTestId("task-state-turn-finished")).toBeNull();
    },
  );
});
