import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { TaskCreatePrioritySelect } from "./task-create-dialog-priority-select";

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

describe("TaskCreatePrioritySelect", () => {
  it("shows the localized label for the current value", () => {
    render(<TaskCreatePrioritySelect value="medium" onChange={vi.fn()} />);
    expect(screen.getByTestId("task-create-priority-select").textContent).toContain("Medium");
  });

  it("offers all four priority tokens and reports a selection", () => {
    const onChange = vi.fn();
    render(<TaskCreatePrioritySelect value="medium" onChange={onChange} />);

    fireEvent.click(screen.getByTestId("task-create-priority-select"));
    const critical = screen.getByTestId("task-create-priority-option-critical");
    fireEvent.click(critical);

    expect(onChange).toHaveBeenCalledWith("critical");
  });
});
