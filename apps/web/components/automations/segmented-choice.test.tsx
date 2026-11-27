import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SegmentedChoice } from "./segmented-choice";

const pointer = vi.hoisted(() => ({ fine: true }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: pointer.fine }),
}));

afterEach(() => {
  cleanup();
  pointer.fine = true;
});

const OPTIONS = [
  { value: "a", label: "Alpha", description: "Alpha runs alone." },
  { value: "b", label: "Beta", description: "Beta runs together." },
] as const;

function renderChoice(value: "a" | "b" = "a") {
  const onValueChange = vi.fn();
  render(
    <TooltipProvider>
      <SegmentedChoice
        id="example"
        label="Example choice"
        value={value}
        options={OPTIONS}
        onValueChange={onValueChange}
      />
    </TooltipProvider>,
  );
  return onValueChange;
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-002.3
describe("SegmentedChoice", () => {
  it("moves the choice with the arrow keys like a radio group", async () => {
    const onValueChange = renderChoice("a");
    const alpha = screen.getByRole("radio", { name: "Alpha" });
    alpha.focus();
    fireEvent.keyDown(alpha, { key: "ArrowRight" });
    // Radix moves roving focus on the next task.
    await waitFor(() => expect(onValueChange).toHaveBeenCalledWith("b"));
  });

  it("exposes the options as named radios in a labelled group", () => {
    renderChoice("b");
    const group = screen.getByRole("radiogroup", { name: "Example choice" });
    expect(group).toBeTruthy();
    expect(screen.getByRole("radio", { name: "Alpha" }).getAttribute("aria-checked")).toBe("false");
    expect(screen.getByRole("radio", { name: "Beta" }).getAttribute("aria-checked")).toBe("true");
  });

  it("describes only the selected option and links the description to the group", () => {
    renderChoice("a");
    const description = screen.getByText("Alpha runs alone.");
    expect(screen.queryByText("Beta runs together.")).toBeNull();
    const group = screen.getByRole("radiogroup", { name: "Example choice" });
    expect(group.getAttribute("aria-describedby")).toBe(description.id);
  });

  it("reports a newly chosen option", () => {
    const onValueChange = renderChoice("a");
    fireEvent.click(screen.getByRole("radio", { name: "Beta" }));
    expect(onValueChange).toHaveBeenCalledWith("b");
  });

  it("keeps a choice when the selected option is pressed again", () => {
    const onValueChange = renderChoice("a");
    fireEvent.click(screen.getByRole("radio", { name: "Alpha" }));
    expect(onValueChange).not.toHaveBeenCalled();
  });

  it("never opens a tooltip for the selected option, whose line is already shown", async () => {
    renderChoice("a");
    fireEvent.focus(screen.getByRole("radio", { name: "Alpha" }));
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("describes another option in a tooltip on keyboard focus", async () => {
    renderChoice("a");
    fireEvent.focus(screen.getByRole("radio", { name: "Beta" }));
    expect((await screen.findByRole("tooltip")).textContent).toContain("Beta runs together.");
  });

  it("does not reopen a tooltip on the option that was selected before", async () => {
    const { rerender } = render(
      <TooltipProvider>
        <SegmentedChoice
          id="stale"
          label="Stale choice"
          value="b"
          options={OPTIONS}
          onValueChange={vi.fn()}
        />
      </TooltipProvider>,
    );
    fireEvent.focus(screen.getAllByRole("radio", { name: "Beta" })[0]);
    rerender(
      <TooltipProvider>
        <SegmentedChoice
          id="stale"
          label="Stale choice"
          value="a"
          options={OPTIONS}
          onValueChange={vi.fn()}
        />
      </TooltipProvider>,
    );
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("leaves touch devices to the selected-option line instead of tooltips", () => {
    pointer.fine = false;
    renderChoice("a");
    fireEvent.focus(screen.getByRole("radio", { name: "Beta" }));
    expect(screen.queryByRole("tooltip")).toBeNull();
  });
});
