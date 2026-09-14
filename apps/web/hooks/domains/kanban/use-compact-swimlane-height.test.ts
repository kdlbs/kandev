import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useCompactSwimlaneHeight } from "./use-compact-swimlane-height";

const steps = ["a", "b"].map((id) => ({ id, title: id, color: "" }));

describe("compact swimlane sizing", () => {
  // @covers AC-UI-ADAPTIVE-KANBAN-002.1, AC-UI-ADAPTIVE-KANBAN-002.5
  it("uses the tallest current column and discards removed measurements", () => {
    const { result, rerender } = renderHook(
      ({ visible }) => useCompactSwimlaneHeight(true, visible, false),
      { initialProps: { visible: steps } },
    );
    expect(result.current.columnHeight).toBe("clamp(12.5rem, 0px, 25rem)");
    const report = result.current.onNaturalHeightChange!;
    act(() => {
      report("a", 270);
      report("b", 600);
    });
    expect(result.current.columnHeight).toBe("clamp(12.5rem, 600px, 25rem)");
    expect(result.current.onNaturalHeightChange).toBe(report);
    rerender({ visible: [steps[0]] });
    expect(result.current.columnHeight).toBe("clamp(12.5rem, 270px, 25rem)");
    act(() => report("b", 600));
    rerender({ visible: steps });
    expect(result.current.columnHeight).toBe("clamp(12.5rem, 270px, 25rem)");
    act(() => report("a", 220));
    expect(result.current.columnHeight).toBe("clamp(12.5rem, 220px, 25rem)");
  });

  it("freezes during drag and applies pending heights after cancellation", () => {
    const { result, rerender } = renderHook(
      ({ dragging }) => useCompactSwimlaneHeight(true, steps, dragging),
      { initialProps: { dragging: false } },
    );
    act(() => result.current.onNaturalHeightChange!("a", 270));
    rerender({ dragging: true });
    act(() => result.current.onNaturalHeightChange!("a", 350));
    expect(result.current.columnHeight).toBe("clamp(12.5rem, 270px, 25rem)");
    rerender({ dragging: false });
    expect(result.current.columnHeight).toBe("clamp(12.5rem, 350px, 25rem)");
  });

  it("disables reporting and clears measurements outside compact mode", () => {
    const { result, rerender } = renderHook(
      ({ enabled }) => useCompactSwimlaneHeight(enabled, steps, false),
      { initialProps: { enabled: true } },
    );
    act(() => result.current.onNaturalHeightChange!("a", 350));
    const lateReport = result.current.onNaturalHeightChange!;
    rerender({ enabled: false });
    expect(result.current.columnHeight).toBeUndefined();
    expect(result.current.onNaturalHeightChange).toBeUndefined();
    act(() => lateReport("a", 350));
    rerender({ enabled: true });
    expect(result.current.columnHeight).toBe("clamp(12.5rem, 0px, 25rem)");
  });
});
