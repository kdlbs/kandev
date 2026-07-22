import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { PlanSelectionPopover } from "./plan-selection-popover";

afterEach(cleanup);

describe("PlanSelectionPopover", () => {
  it("keeps entered feedback open when the submitter rejects the save", () => {
    const onClose = vi.fn();
    render(
      <PlanSelectionPopover
        selectedText="settled answer"
        position={{ x: 100, y: 100 }}
        onAdd={() => false}
        onClose={onClose}
      />,
    );

    const input = screen.getByPlaceholderText("Add your comment or instruction...");
    fireEvent.change(input, { target: { value: "Keep this feedback" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    expect(onClose).not.toHaveBeenCalled();
    expect((input as HTMLTextAreaElement).value).toBe("Keep this feedback");
  });
});
