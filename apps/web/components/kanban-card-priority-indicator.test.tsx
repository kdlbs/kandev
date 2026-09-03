import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { KanbanCardPriorityIndicator } from "./kanban-card-priority-indicator";

afterEach(cleanup);

describe("KanbanCardPriorityIndicator", () => {
  it.each(["critical", "high", "low"] as const)("renders an indicator for %s", (priority) => {
    render(<KanbanCardPriorityIndicator priority={priority} />);
    expect(screen.getByTestId("kanban-card-priority-indicator")).not.toBeNull();
  });

  it("renders nothing for medium, the default and majority case", () => {
    const { container } = render(<KanbanCardPriorityIndicator priority="medium" />);
    expect(container.querySelector('[data-testid="kanban-card-priority-indicator"]')).toBeNull();
    expect(container.textContent).toBe("");
  });

  it.each([undefined, null, "", "urgent"])(
    "renders nothing and no raw text for an unrecognized value (%s)",
    (priority) => {
      const { container } = render(<KanbanCardPriorityIndicator priority={priority} />);
      expect(container.querySelector('[data-testid="kanban-card-priority-indicator"]')).toBeNull();
      expect(container.textContent).toBe("");
    },
  );

  it("renders a visually distinct treatment for each of critical, high and low", () => {
    const { container: critical } = render(<KanbanCardPriorityIndicator priority="critical" />);
    const { container: high } = render(<KanbanCardPriorityIndicator priority="high" />);
    const { container: low } = render(<KanbanCardPriorityIndicator priority="low" />);

    // Distinct icons (not merely distinct colors) satisfy AC-001.4.
    expect(critical.querySelector("svg")?.outerHTML).not.toBe(high.querySelector("svg")?.outerHTML);
    expect(high.querySelector("svg")?.outerHTML).not.toBe(low.querySelector("svg")?.outerHTML);
    expect(critical.querySelector("svg")).not.toBeNull();
    expect(high.querySelector("svg")).not.toBeNull();
    expect(low.querySelector("svg")).not.toBeNull();
  });

  it("gives the indicator an accessible name including the localized label", () => {
    render(<KanbanCardPriorityIndicator priority="critical" />);
    expect(screen.getByRole("img", { name: /Critical/i })).not.toBeNull();
  });
});
