"use client";

import { memo, useCallback, useMemo, useState } from "react";
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
import type { RichOutputChartBlock } from "./types";

const HOST_SERIES_COLORS = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
] as const;
const CHART_MARGIN = { left: 0, right: 8 } as const;
const BAR_RADIUS: [number, number, number, number] = [3, 3, 0, 0];

function seriesKey(index: number): string {
  return `series_${index}`;
}

function chartConfig(block: RichOutputChartBlock): ChartConfig {
  return Object.fromEntries(
    block.series.map((series, index) => [
      seriesKey(index),
      { label: series.label, color: HOST_SERIES_COLORS[index] },
    ]),
  );
}

function chartData(block: RichOutputChartBlock): Array<Record<string, string | number | null>> {
  return block.labels.map((label, labelIndex) => {
    const row: Record<string, string | number | null> = { label };
    for (let seriesIndex = 0; seriesIndex < block.series.length; seriesIndex += 1) {
      row[seriesKey(seriesIndex)] = block.series[seriesIndex].values[labelIndex];
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
  block,
  hiddenSeries,
  onToggle,
}: {
  block: RichOutputChartBlock;
  hiddenSeries: ReadonlySet<string>;
  onToggle: (key: string) => void;
}) {
  const isInteractive = block.series.length > 1;

  return (
    <div className="flex flex-wrap items-center justify-center gap-x-1 gap-y-0 pt-1">
      {block.series.map((series, index) => {
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
            className={`flex min-h-11 cursor-pointer items-center gap-1.5 rounded-md px-2 text-[11px] text-muted-foreground transition-[opacity,transform] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 active:scale-[0.98] ${isVisible ? "opacity-100" : "opacity-50"}`}
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

function useChartPresentation(block: RichOutputChartBlock) {
  const { i18n } = useTranslation();
  const [hiddenSeries, setHiddenSeries] = useState<ReadonlySet<string>>(() => new Set());
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const formatXAxisTick = useMemo(
    () => createXAxisTickFormatter(block.labels, locale),
    [block.labels, locale],
  );
  const formatYAxisValue = useCallback(
    (value: unknown) => formatYAxisTick(value, locale),
    [locale],
  );
  const toggleSeries = useCallback((key: string) => {
    setHiddenSeries((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  return { formatXAxisTick, formatYAxisValue, hiddenSeries, toggleSeries };
}

export const ChartBlock = memo(function ChartBlock({ block }: { block: RichOutputChartBlock }) {
  const { formatXAxisTick, formatYAxisValue, hiddenSeries, toggleSeries } =
    useChartPresentation(block);
  const { plotRef, shouldMountPlot } = useChartPlotVisibility();
  const shouldAnimate = useRichOutputChartAnimations();
  const data = useMemo(() => chartData(block), [block.labels, block.series]);
  const config = useMemo(() => chartConfig(block), [block.series]);
  const legend = useMemo(
    () => (
      <ChartLegend
        content={<SeriesLegend block={block} hiddenSeries={hiddenSeries} onToggle={toggleSeries} />}
      />
    ),
    [block, hiddenSeries, toggleSeries],
  );

  return (
    <figure className="min-w-0 space-y-3" data-testid={`rich-output-chart-${block.chart_type}`}>
      <figcaption className="space-y-0.5">
        <h4 className="text-xs font-medium text-foreground">{block.title}</h4>
        <p className="text-[11px] leading-relaxed text-muted-foreground">{block.summary}</p>
      </figcaption>
      <div ref={plotRef} className="h-52 min-h-52 w-full min-w-0 max-w-full">
        {shouldMountPlot && (
          <ChartContainer
            config={config}
            className="h-full min-h-52 w-full min-w-0 max-w-full aspect-auto"
            aria-label={block.summary}
          >
            {block.chart_type === "line" ? (
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
                {block.series.map((series, index) => (
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
                {block.series.map((series, index) => (
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
