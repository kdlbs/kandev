"use client";

import { useRichOutputViewState } from "./view-state";

import { memo, useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Bar, BarChart, CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import {
  ChartContainer,
  ChartLegend,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@kandev/ui/chart";
import { createXAxisTickFormatter, formatYAxisTick } from "./chart-format";
import { useRichOutputChartAnimations } from "./chart-motion";
import { useChartPlotVisibility } from "./chart-visibility";
import type { ChartProps } from "@/lib/rich-output/catalog";
import { controlSizingClassName } from "@kandev/ui/control-sizing";

const HOST_SERIES_COLORS = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
] as const;
const CHART_MARGIN = { left: 0, right: 8 } as const;
const BAR_RADIUS: [number, number, number, number] = [3, 3, 0, 0];
// i18n-exempt: CSS layout classes, not user-facing copy.
const CHART_PLOT_CLASS_NAME = "h-52 min-h-52 w-full min-w-0 max-w-full";

function seriesKey(index: number): string {
  return `series_${index}`;
}

function chartConfig(chart: ChartProps): ChartConfig {
  return Object.fromEntries(
    chart.series.map((series, index) => [
      seriesKey(index),
      { label: series.label, color: HOST_SERIES_COLORS[index] },
    ]),
  );
}

function chartData(chart: ChartProps): Array<Record<string, string | number | null>> {
  return chart.labels.map((label, labelIndex) => {
    const row: Record<string, string | number | null> = { label };
    for (let seriesIndex = 0; seriesIndex < chart.series.length; seriesIndex += 1) {
      row[seriesKey(seriesIndex)] = chart.series[seriesIndex].values[labelIndex];
    }
    return row;
  });
}

function SeriesMarker({ color }: { color: string }) {
  return (
    <span
      aria-hidden="true"
      className="h-2 w-2 shrink-0 rounded-[2px]"
      style={{ backgroundColor: color }}
    />
  );
}

function SeriesLegend({
  chart,
  hiddenSeries,
  onToggle,
}: {
  chart: ChartProps;
  hiddenSeries: ReadonlySet<string>;
  onToggle: (key: string) => void;
}) {
  const isInteractive = chart.series.length > 1;

  return (
    <div className="flex flex-wrap items-center justify-center gap-x-1 gap-y-0 pt-1">
      {chart.series.map((series, index) => {
        const key = seriesKey(index);
        const isVisible = !hiddenSeries.has(key);
        const content = (
          <>
            <SeriesMarker color={HOST_SERIES_COLORS[index]} />
            <span className={isVisible ? undefined : "line-through"}>{series.label}</span>
          </>
        );

        return isInteractive ? (
          <button
            key={key}
            type="button"
            aria-pressed={isVisible}
            className={`${controlSizingClassName("standard", "max-md:h-12 [@media(pointer:coarse)]:h-12")} flex cursor-pointer items-center gap-1.5 rounded-md px-2 text-[11px] text-muted-foreground transition-[opacity,transform] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 active:scale-[0.98] ${isVisible ? "opacity-100" : "opacity-50"}`}
            data-testid={`rich-output-chart-legend-${key}`}
            onClick={() => onToggle(key)}
          >
            {content}
          </button>
        ) : (
          <div
            key={key}
            className="flex min-h-7 items-center gap-1.5 px-2 text-[11px] text-muted-foreground"
            data-testid={`rich-output-chart-legend-${key}`}
          >
            {content}
          </div>
        );
      })}
    </div>
  );
}

function useChartPresentation(chart: ChartProps) {
  const { i18n } = useTranslation();
  const [hidden, setHidden] = useRichOutputViewState<string[]>(
    JSON.stringify(chart.series.map((series) => series.label)),
    [],
  );
  const hiddenSeries = useMemo(() => new Set(hidden), [hidden]);
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const formatXAxisTick = useMemo(
    () => createXAxisTickFormatter(chart.labels, locale),
    [chart.labels, locale],
  );
  const formatYAxisValue = useCallback(
    (value: unknown) => formatYAxisTick(value, locale),
    [locale],
  );
  const toggleSeries = useCallback(
    (key: string) => {
      setHidden((current) => {
        const next = new Set(current);
        if (next.has(key)) next.delete(key);
        else next.add(key);
        return [...next];
      });
    },
    [setHidden],
  );

  const legend = useMemo(
    () => (
      <ChartLegend
        content={<SeriesLegend chart={chart} hiddenSeries={hiddenSeries} onToggle={toggleSeries} />}
      />
    ),
    [chart, hiddenSeries, toggleSeries],
  );

  return { formatXAxisTick, formatYAxisValue, hiddenSeries, legend };
}

export const Chart = memo(function Chart(chart: ChartProps) {
  const { formatXAxisTick, formatYAxisValue, hiddenSeries, legend } = useChartPresentation(chart);
  const { plotRef, shouldMountPlot } = useChartPlotVisibility();
  const shouldAnimate = useRichOutputChartAnimations();
  const data = useMemo(() => chartData(chart), [chart.labels, chart.series]);
  const config = useMemo(() => chartConfig(chart), [chart.series]);

  return (
    <figure className="min-w-0 space-y-3" data-testid={`rich-output-chart-${chart.chart_type}`}>
      <figcaption className="space-y-0.5">
        <h4 className="text-xs font-medium text-foreground">{chart.title}</h4>
        <p className="text-[11px] leading-relaxed text-muted-foreground">{chart.summary}</p>
      </figcaption>
      <div ref={plotRef} data-testid="rich-output-chart-plot" className={CHART_PLOT_CLASS_NAME}>
        {shouldMountPlot && (
          <ChartContainer
            config={config}
            className="h-full min-h-52 w-full min-w-0 max-w-full aspect-auto"
            aria-label={chart.summary}
          >
            {chart.chart_type === "line" ? (
              <LineChart data={data} accessibilityLayer margin={CHART_MARGIN}>
                <CartesianGrid vertical={false} />
                <XAxis
                  dataKey="label"
                  tickFormatter={formatXAxisTick}
                  tickLine={false}
                  axisLine={false}
                  minTickGap={20}
                  tickMargin={8}
                />
                <YAxis
                  tickFormatter={formatYAxisValue}
                  tickLine={false}
                  axisLine={false}
                  tickMargin={4}
                  width={48}
                />
                <ChartTooltip content={<ChartTooltipContent />} />
                {chart.series.map((series, index) => (
                  <Line
                    key={`${series.label}-${index}`}
                    dataKey={seriesKey(index)}
                    type="monotone"
                    stroke={`var(--color-${seriesKey(index)})`}
                    strokeWidth={2}
                    dot={false}
                    connectNulls={false}
                    isAnimationActive={shouldAnimate}
                    hide={hiddenSeries.has(seriesKey(index))}
                  />
                ))}
                {legend}
              </LineChart>
            ) : (
              <BarChart data={data} accessibilityLayer margin={CHART_MARGIN}>
                <CartesianGrid vertical={false} />
                <XAxis
                  dataKey="label"
                  tickFormatter={formatXAxisTick}
                  tickLine={false}
                  axisLine={false}
                  minTickGap={20}
                  tickMargin={8}
                />
                <YAxis
                  tickFormatter={formatYAxisValue}
                  tickLine={false}
                  axisLine={false}
                  tickMargin={4}
                  width={48}
                />
                <ChartTooltip content={<ChartTooltipContent />} />
                {chart.series.map((series, index) => (
                  <Bar
                    key={`${series.label}-${index}`}
                    dataKey={seriesKey(index)}
                    fill={`var(--color-${seriesKey(index)})`}
                    radius={BAR_RADIUS}
                    isAnimationActive={shouldAnimate}
                    hide={hiddenSeries.has(seriesKey(index))}
                  />
                ))}
                {legend}
              </BarChart>
            )}
          </ChartContainer>
        )}
      </div>
    </figure>
  );
});
