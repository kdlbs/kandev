import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { TaskCreatePrioritySelect } from "./task-create-dialog-priority-select";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) =>
      ({
        "kanban:priority": "Priority",
        "kanban:priorityCritical": "Critical",
        "kanban:priorityHigh": "High",
        "kanban:priorityMedium": "Medium",
        "kanban:priorityLow": "Low",
        "task:priorityInfoLabel": "About task priority",
        "task:priorityInfo": "Priority shows how urgent this task is on the board.",
      })[key] ?? key,
  }),
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => false,
}));

beforeAll(() => {
  // Radix Select needs these in jsdom; the repo does not otherwise polyfill them.
  if (!Element.prototype.hasPointerCapture) {
    Element.prototype.hasPointerCapture = () => false;
  }
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {};
  }
});

afterEach(cleanup);

function renderPrioritySelect(props: React.ComponentProps<typeof TaskCreatePrioritySelect>) {
  return render(
    <TooltipProvider>
      <TaskCreatePrioritySelect {...props} />
    </TooltipProvider>,
  );
}

describe("TaskCreatePrioritySelect", () => {
  it("shows the localized label for the current value", () => {
    renderPrioritySelect({ value: "medium", onChange: vi.fn() });
    expect(screen.getByTestId("task-create-priority-select").textContent).toContain("Medium");
  });

  it("offers all four priority tokens and reports a selection", () => {
    const onChange = vi.fn();
    renderPrioritySelect({ value: "medium", onChange });

    fireEvent.click(screen.getByTestId("task-create-priority-select"));
    const critical = screen.getByTestId("task-create-priority-option-critical");
    fireEvent.click(critical);

    expect(onChange).toHaveBeenCalledWith("critical");
  });

  it("matches dialog selector styling and explains the setting", async () => {
    renderPrioritySelect({ value: "medium", onChange: vi.fn() });

    const select = screen.getByTestId("task-create-priority-select");
    expect(select.className).toContain("bg-muted/30");

    const info = screen.getByTestId("task-create-priority-setting-info");
    expect(info.getAttribute("aria-label")).toBe("About task priority");

    fireEvent.focus(info);
    await waitFor(() => {
      expect(screen.getByRole("tooltip").textContent).toContain(
        "Priority shows how urgent this task is on the board.",
      );
    });
  });
});
