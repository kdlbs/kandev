import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, renderHook, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import {
  SelectionActionSlot,
  SelectionActionSlotProvider,
  useSelectionActionSlot,
} from "./selection-action-slot";

afterEach(cleanup);

describe("SelectionActionSlot", () => {
  it("publishes the mounted slot element to selection surfaces in the same provider", () => {
    function Probe() {
      const slot = useSelectionActionSlot();
      return <span data-testid="probe">{slot ? slot.getAttribute("data-testid") : "none"}</span>;
    }
    render(
      <SelectionActionSlotProvider>
        <Probe />
        <div data-testid="toolbar-row">
          <SelectionActionSlot />
        </div>
      </SelectionActionSlotProvider>,
    );

    const slot = screen.getByTestId("selection-action-slot");
    expect(screen.getByTestId("toolbar-row").contains(slot)).toBe(true);
    expect(screen.getByTestId("probe").textContent).toBe("selection-action-slot");
    // Pressing a slotted action must not drop the tracked touch selection.
    expect(slot.hasAttribute("data-selection-action-dock")).toBe(true);
  });

  it("renders nothing and reports no slot without a provider", () => {
    const { container } = render(<SelectionActionSlot />);
    expect(container.childElementCount).toBe(0);
    const { result } = renderHook(() => useSelectionActionSlot());
    expect(result.current).toBeNull();
  });

  it("stops publishing the slot after it unmounts", () => {
    const wrapper = ({ children }: { children: ReactNode }) => (
      <SelectionActionSlotProvider>{children}</SelectionActionSlotProvider>
    );
    function Harness({ showSlot }: { showSlot: boolean }) {
      const slot = useSelectionActionSlot();
      return (
        <>
          <span data-testid="has-slot">{String(Boolean(slot))}</span>
          {showSlot ? <SelectionActionSlot /> : null}
        </>
      );
    }
    const { rerender } = render(<Harness showSlot />, { wrapper });
    expect(screen.getByTestId("has-slot").textContent).toBe("true");
    rerender(<Harness showSlot={false} />);
    expect(screen.getByTestId("has-slot").textContent).toBe("false");
  });
});
