/**
 * Custom-SVG-style stacked bar chart primitive shared by every
 * chart on the agent dashboard. There is no chart library; the
 * renderer is just nested flex divs with data attributes pinned
 * for tests. Each "bar" is a vertical stack of segments whose
 * heights are proportional to their counts; the maximum-bar
 * height drives the y-axis.
 *
 * Two reasons for the custom approach:
 * 1. We render this both server-side (for SEO + zero-JS hydration)
 *    AND client-side; a chart library that requires runtime
 *    measurements would force a `useEffect` indirection that
 *    breaks the SSR pass.
 * 2. The bars need to be deeply test-pinned via `data-testid`
 *    attributes — Playwright walks the segments and asserts the
 *    counts directly. A library's DOM is unstable across versions.
 */

import type { CSSProperties } from "react";
import { cn } from "@/lib/utils";

export type StackedBarSegment = {
  /** Sub-bucket key, e.g. "succeeded" / "failed". */
  key: string;
  /** Numeric count for this segment. */
  value: number;
  /** Tailwind background class for the segment. */
  className: string;
};

export type StackedBarRow = {
  /** Stable identifier passed back via data-bar-id. */
  id: string;
  /** Caption rendered under the bar (e.g. the date). */
  label: string;
  /** Optional second-line caption (e.g. day of week). */
  sublabel?: string;
  /** Stacked segments, top-to-bottom in the order supplied. */
  segments: StackedBarSegment[];
};

export type StackedBarsProps = {
  /** One entry per bar. Order is preserved. */
  rows: StackedBarRow[];
  /** Total chart height in pixels. */
  heightPx?: number;
  /** Locks the y-axis when known (e.g. to align two charts). */
  maxValue?: number;
  /** Optional aria label for the chart container. */
  ariaLabel?: string;
  /** When true, hides the bar labels (used by sparkline-style charts). */
  hideLabels?: boolean;
};

const DEFAULT_HEIGHT_PX = 140;

/**
 * Returns the largest sum across all bars, or 1 when every row is
 * empty (so we never divide by zero in the height calculation).
 */
function computeMaxValue(rows: StackedBarRow[]): number {
  let max = 1;
  for (const row of rows) {
    let total = 0;
    for (const seg of row.segments) {
      total += seg.value;
    }
    if (total > max) max = total;
  }
  return max;
}

/**
 * Stable, test-pinned stacked bar chart. Each bar is a column of
 * absolutely-stacked segments inside a fixed-height container.
 * Segments with value=0 are still rendered (as zero-height) so
 * snapshot tests can rely on a fixed segment count per bar.
 */
export function StackedBars({
  rows,
  heightPx = DEFAULT_HEIGHT_PX,
  maxValue,
  ariaLabel,
  hideLabels,
}: StackedBarsProps) {
  const max = maxValue ?? computeMaxValue(rows);
  const containerStyle: CSSProperties = { height: `${heightPx}px` };

  return (
    <div className="w-full" data-testid="stacked-bars" aria-label={ariaLabel}>
      <div
        className="flex items-end gap-[2px] w-full border-b border-border/40"
        style={containerStyle}
      >
        {rows.map((row) => (
          <Bar key={row.id} row={row} heightPx={heightPx} max={max} />
        ))}
      </div>
      {!hideLabels && (
        <div className="flex gap-[2px] w-full mt-1" data-testid="stacked-bars-labels">
          {rows.map((row) => (
            <div
              key={row.id}
              className="flex-1 min-w-0 text-[10px] text-muted-foreground text-center"
              data-bar-id={row.id}
            >
              <div className="truncate">{row.label}</div>
              {row.sublabel ? <div className="truncate opacity-70">{row.sublabel}</div> : null}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

/**
 * Renders one column. Width is split evenly across the parent flex
 * container; height of each segment is `(value / max) * heightPx`
 * truncated to integer pixels for crisp rendering.
 */
function Bar({ row, heightPx, max }: { row: StackedBarRow; heightPx: number; max: number }) {
  const total = row.segments.reduce((sum, seg) => sum + seg.value, 0);
  return (
    <div
      className="flex-1 min-w-0 flex flex-col-reverse h-full"
      data-testid="stacked-bar"
      data-bar-id={row.id}
      data-bar-total={total}
      title={`${row.label}: ${total}`}
    >
      {row.segments.map((seg) => {
        const px = max > 0 ? Math.floor((seg.value / max) * heightPx) : 0;
        return (
          <div
            key={seg.key}
            className={cn(seg.className, "w-full")}
            style={{ height: `${px}px` }}
            data-segment-key={seg.key}
            data-segment-value={seg.value}
            data-segment-px={px}
          />
        );
      })}
    </div>
  );
}
