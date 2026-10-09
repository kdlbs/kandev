import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
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
  it.each(["onSelect", "onClick"] as const)(
    "blocks pending row activation and restores %s after failure",
    (handler) => {
      const activate = vi.fn();
      const rowView = (pending: boolean) => (
        <StateProvider>
          <TooltipProvider>
            <TaskItem
              title="Delete target"
              state="REVIEW"
              isPendingRemoval={pending}
              {...{ [handler]: activate }}
            />
          </TooltipProvider>
        </StateProvider>
      );
      const { rerender } = render(rowView(true));
      const row = screen.getByTestId("sidebar-task-item");
      fireEvent.click(row);
      fireEvent.keyDown(row, { key: "Enter" });
      fireEvent.keyDown(row, { key: " " });
      expect(activate).not.toHaveBeenCalled();
      rerender(rowView(false));
      fireEvent.click(row);
      fireEvent.keyDown(row, { key: "Enter" });
      expect(activate).toHaveBeenCalledTimes(2);
    },
  );
});
