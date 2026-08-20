import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RichOutputChartBlock } from "./types";

type ObserverRecord = {
  callback: IntersectionObserverCallback;
  instance: IntersectionObserver;
  options?: IntersectionObserverInit;
  target?: Element;
};

const observerRecords: ObserverRecord[] = [];
const visibilityDescriptor = Object.getOwnPropertyDescriptor(document, "visibilityState");
const MOCK_LINE_CHART = "mock-line-chart";
const chartMotion = vi.hoisted(() => ({ enabled: true }));
const captured = vi.hoisted(() => ({
  bars: [] as Array<Record<string, unknown>>,
  chartContainers: [] as Array<Record<string, unknown>>,
  lineCharts: [] as Array<Record<string, unknown>>,
  lines: [] as Array<Record<string, unknown>>,
  xAxes: [] as Array<Record<string, unknown>>,
  yAxes: [] as Array<Record<string, unknown>>,
}));

vi.mock("./chart-motion", () => ({
  useRichOutputChartAnimations: () => chartMotion.enabled,
}));

vi.mock("@kandev/ui/chart", () => ({
  ChartContainer: (props: { children: ReactNode }) => {
    captured.chartContainers.push(props);
    return <div>{props.children}</div>;
  },
  ChartLegend: ({ content }: { content: ReactNode }) => <>{content}</>,
  ChartTooltip: () => null,
  ChartTooltipContent: () => null,
}));

vi.mock("recharts", () => ({
  Bar: (props: Record<string, unknown>) => {
    captured.bars.push(props);
    return null;
  },
  BarChart: ({ children }: { children: ReactNode }) => (
    <div data-testid="mock-bar-chart">{children}</div>
  ),
  CartesianGrid: () => null,
  Line: (props: Record<string, unknown>) => {
    captured.lines.push(props);
    return null;
  },
  LineChart: (props: { children: ReactNode }) => {
    captured.lineCharts.push(props);
    return <div data-testid="mock-line-chart">{props.children}</div>;
  },
  XAxis: (props: Record<string, unknown>) => {
    captured.xAxes.push(props);
    return null;
  },
  YAxis: (props: Record<string, unknown>) => {
    captured.yAxes.push(props);
    return null;
  },
}));

import { ChartBlock } from "./chart-block";

const BLOCK: RichOutputChartBlock = {
  type: "chart",
  chart_type: "line",
  title: "Latency",
  summary: "Latency over time",
  labels: ["Mon", "Tue"],
  series: [{ label: "p95", values: [120, 110] }],
};

function installIntersectionObserver() {
  class MockIntersectionObserver implements IntersectionObserver {
    readonly root = null;
    readonly rootMargin = "0px";
    readonly thresholds: readonly number[] = [];
    private readonly record: ObserverRecord;

    constructor(callback: IntersectionObserverCallback, options?: IntersectionObserverInit) {
      this.record = { callback, instance: this, options };
      observerRecords.push(this.record);
    }

    disconnect() {}
    observe(target: Element) {
      this.record.target = target;
    }
    takeRecords(): IntersectionObserverEntry[] {
      return [];
    }
    unobserve() {}
  }

  vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
}

function intersect(record = observerRecords[0]) {
  act(() => {
    record.callback(
      [{ isIntersecting: true, target: record.target } as IntersectionObserverEntry],
      record.instance,
    );
  });
}

function setDocumentVisibility(value: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", { configurable: true, value });
  document.dispatchEvent(new Event("visibilitychange"));
}

beforeEach(() => {
  observerRecords.length = 0;
  for (const values of Object.values(captured)) values.length = 0;
  installIntersectionObserver();
  chartMotion.enabled = true;
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  if (visibilityDescriptor) {
    Object.defineProperty(document, "visibilityState", visibilityDescriptor);
  } else {
    Reflect.deleteProperty(document, "visibilityState");
  }
});

describe("ChartBlock plot scheduling", () => {
  it("defers the Recharts plot until the chart approaches the viewport", () => {
    render(<ChartBlock block={BLOCK} />);

    expect(screen.queryByTestId(MOCK_LINE_CHART)).toBeNull();
    expect(screen.getByText(BLOCK.title)).not.toBeNull();
    expect(observerRecords[0].options?.rootMargin).toBe("200px 0px");

    intersect();
    expect(screen.getByTestId(MOCK_LINE_CHART)).not.toBeNull();
  });

  it("waits for a background tab to become visible after intersection", () => {
    setDocumentVisibility("hidden");
    render(<ChartBlock block={BLOCK} />);

    intersect();
    expect(screen.queryByTestId(MOCK_LINE_CHART)).toBeNull();

    act(() => setDocumentVisibility("visible"));
    expect(screen.getByTestId(MOCK_LINE_CHART)).not.toBeNull();
  });

  it("keeps an eligible plot mounted after it leaves the visible tab", () => {
    render(<ChartBlock block={BLOCK} />);
    intersect();
    const mountedPlot = screen.getByTestId(MOCK_LINE_CHART);

    act(() => setDocumentVisibility("hidden"));

    expect(screen.getByTestId(MOCK_LINE_CHART)).toBe(mountedPlot);
  });

  it("renders immediately when intersection observation is unavailable", () => {
    vi.stubGlobal("IntersectionObserver", undefined);

    render(<ChartBlock block={BLOCK} />);

    expect(screen.getByTestId(MOCK_LINE_CHART)).not.toBeNull();
  });

  it("reuses derived chart inputs when a legend toggle changes local visibility", () => {
    const block: RichOutputChartBlock = {
      ...BLOCK,
      series: [BLOCK.series[0], { label: "p50", values: [80, 75] }],
    };
    render(<ChartBlock block={block} />);
    intersect();

    const initialData = captured.lineCharts.at(-1)?.data;
    const initialConfig = captured.chartContainers.at(-1)?.config;
    const initialXAxisFormatter = captured.xAxes.at(-1)?.tickFormatter;
    const initialYAxisFormatter = captured.yAxes.at(-1)?.tickFormatter;

    fireEvent.click(screen.getByRole("button", { name: "p50" }));

    expect(captured.lineCharts.at(-1)?.data).toBe(initialData);
    expect(captured.chartContainers.at(-1)?.config).toBe(initialConfig);
    expect(captured.xAxes.at(-1)?.tickFormatter).toBe(initialXAxisFormatter);
    expect(captured.yAxes.at(-1)?.tickFormatter).toBe(initialYAxisFormatter);
    expect(captured.lines.at(-1)?.hide).toBe(true);
  });

  it("skips an unchanged parent rerender and retains default chart animation", () => {
    const view = render(<ChartBlock block={BLOCK} />);
    intersect();
    const renderCount = captured.lineCharts.length;

    view.rerender(<ChartBlock block={BLOCK} />);

    expect(captured.lineCharts).toHaveLength(renderCount);
    expect(captured.lines.at(-1)?.isAnimationActive).not.toBe(false);
  });

  it("disables every line and bar animation when effective chart motion is off", () => {
    chartMotion.enabled = false;
    const line = render(<ChartBlock block={BLOCK} />);
    intersect();

    expect(captured.lines.at(-1)?.isAnimationActive).toBe(false);

    line.unmount();
    observerRecords.length = 0;
    const barBlock: RichOutputChartBlock = { ...BLOCK, chart_type: "bar" };
    render(<ChartBlock block={barBlock} />);
    intersect();

    expect(captured.bars.at(-1)?.isAnimationActive).toBe(false);
  });
});
