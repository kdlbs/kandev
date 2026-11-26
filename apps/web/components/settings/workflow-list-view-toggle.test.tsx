import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkflowListViewToggle } from "./workflow-list-view-toggle";

afterEach(cleanup);

describe("WorkflowListViewToggle", () => {
  it("renders a labelled group with the current view selected", () => {
    render(<WorkflowListViewToggle view="compact" onViewChange={vi.fn()} />);
    expect(screen.getByRole("group", { name: "Workflow list view" })).toBeTruthy();
    expect(screen.getByTestId("workflow-list-view-compact").getAttribute("data-state")).toBe("on");
    expect(screen.getByTestId("workflow-list-view-detailed").getAttribute("data-state")).toBe(
      "off",
    );
  });

  it("switches to the other view", () => {
    const onViewChange = vi.fn();
    render(<WorkflowListViewToggle view="compact" onViewChange={onViewChange} />);
    fireEvent.click(screen.getByText("Detailed"));
    expect(onViewChange).toHaveBeenCalledWith("detailed");
  });

  it("keeps one view selected when the active item is clicked again", () => {
    const onViewChange = vi.fn();
    render(<WorkflowListViewToggle view="compact" onViewChange={onViewChange} />);
    fireEvent.click(screen.getByText("Compact"));
    expect(onViewChange).not.toHaveBeenCalled();
  });
});
