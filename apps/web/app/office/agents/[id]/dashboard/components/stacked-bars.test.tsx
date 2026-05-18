import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { StackedBars, type StackedBarRow } from "./stacked-bars";

afterEach(() => cleanup());

const HEIGHT = 100;
const SEGMENT_PX_ATTR = "data-segment-px";

function rows(): StackedBarRow[] {
  return [
    {
      id: "d1",
      label: "Apr 21",
      segments: [
        { key: "succeeded", value: 3, className: "bg-emerald-500" },
        { key: "failed", value: 1, className: "bg-red-500" },
      ],
    },
    {
      id: "d2",
      label: "Apr 22",
      segments: [
        { key: "succeeded", value: 0, className: "bg-emerald-500" },
        { key: "failed", value: 2, className: "bg-red-500" },
      ],
    },
  ];
}

describe("StackedBars", () => {
  it("renders one bar per row with per-segment values pinned", () => {
    render(<StackedBars rows={rows()} heightPx={HEIGHT} />);
    const bars = screen.getAllByTestId("stacked-bar");
    expect(bars).toHaveLength(2);
    expect(bars[0]?.getAttribute("data-bar-id")).toBe("d1");
    expect(bars[0]?.getAttribute("data-bar-total")).toBe("4");
    expect(bars[1]?.getAttribute("data-bar-id")).toBe("d2");
    expect(bars[1]?.getAttribute("data-bar-total")).toBe("2");
  });

  it("scales segment height by max value across all bars", () => {
    render(<StackedBars rows={rows()} heightPx={HEIGHT} />);
    const bars = screen.getAllByTestId("stacked-bar");
    // d1 has total 4, which is the chart max — so its segments should
    // occupy a proportional share of the 100px height. succeeded=3
    // → floor(3/4 * 100) = 75; failed=1 → floor(1/4 * 100) = 25.
    const d1Succeeded = bars[0]?.querySelector('[data-segment-key="succeeded"]');
    const d1Failed = bars[0]?.querySelector('[data-segment-key="failed"]');
    expect(d1Succeeded?.getAttribute(SEGMENT_PX_ATTR)).toBe("75");
    expect(d1Failed?.getAttribute(SEGMENT_PX_ATTR)).toBe("25");

    // d2 has total 2 (succeeded=0, failed=2). Its succeeded should be
    // 0px, its failed = floor(2/4 * 100) = 50px.
    const d2Succeeded = bars[1]?.querySelector('[data-segment-key="succeeded"]');
    const d2Failed = bars[1]?.querySelector('[data-segment-key="failed"]');
    expect(d2Succeeded?.getAttribute(SEGMENT_PX_ATTR)).toBe("0");
    expect(d2Failed?.getAttribute(SEGMENT_PX_ATTR)).toBe("50");
  });

  it("renders bar labels by default", () => {
    render(<StackedBars rows={rows()} heightPx={HEIGHT} />);
    expect(screen.getByText("Apr 21")).toBeTruthy();
  });

  it("hides bar labels when hideLabels is set", () => {
    render(<StackedBars rows={rows()} heightPx={HEIGHT} hideLabels />);
    expect(screen.queryByText("Apr 21")).toBeNull();
  });

  it("uses provided maxValue when supplied", () => {
    // Forcing max=10 makes succeeded=3 → floor(30/100 * 100) = 30.
    render(<StackedBars rows={rows()} heightPx={HEIGHT} maxValue={10} />);
    const succeeded = screen
      .getAllByTestId("stacked-bar")[0]
      ?.querySelector('[data-segment-key="succeeded"]');
    expect(succeeded?.getAttribute(SEGMENT_PX_ATTR)).toBe("30");
  });
});
