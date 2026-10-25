import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TaskPriorityIndicator } from "./task-priority-indicator";

afterEach(cleanup);

describe("TaskPriorityIndicator", () => {
  it.each(["critical", "high", "low"] as const)("renders an indicator for %s", (priority) => {
    render(<TaskPriorityIndicator priority={priority} />);
    expect(screen.getByTestId("task-priority-indicator")).not.toBeNull();
  });

  it("renders nothing for medium, the default and majority case", () => {
    const { container } = render(<TaskPriorityIndicator priority="medium" />);
    expect(container.querySelector('[data-testid="task-priority-indicator"]')).toBeNull();
    expect(container.textContent).toBe("");
  });

  it.each([undefined, null, "", "urgent"])(
    "renders nothing and no raw text for an unrecognized value (%s)",
    (priority) => {
      const { container } = render(<TaskPriorityIndicator priority={priority} />);
      expect(container.querySelector('[data-testid="task-priority-indicator"]')).toBeNull();
      expect(container.textContent).toBe("");
    },
  );

  it("renders a visually distinct treatment for each of critical, high and low", () => {
    const { container: critical } = render(<TaskPriorityIndicator priority="critical" />);
    const { container: high } = render(<TaskPriorityIndicator priority="high" />);
    const { container: low } = render(<TaskPriorityIndicator priority="low" />);

    expect(critical.querySelector("svg")?.outerHTML).not.toBe(high.querySelector("svg")?.outerHTML);
    expect(high.querySelector("svg")?.outerHTML).not.toBe(low.querySelector("svg")?.outerHTML);
    expect(critical.querySelector("svg")).not.toBeNull();
    expect(high.querySelector("svg")).not.toBeNull();
    expect(low.querySelector("svg")).not.toBeNull();
  });

  it("gives the indicator an accessible name including the localized label", () => {
    render(<TaskPriorityIndicator priority="critical" />);
    expect(screen.getByRole("img", { name: /Critical/i })).not.toBeNull();
  });
});
