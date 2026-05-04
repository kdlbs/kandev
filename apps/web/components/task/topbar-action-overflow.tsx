"use client";

import { useCallback, useLayoutEffect, useRef, useState, type ReactNode, type Ref } from "react";
import { IconDots } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@kandev/ui/lib/utils";

export type TopbarOverflowItem = {
  id: string;
  label: string;
  priority: number;
  content: ReactNode;
};

type TopbarOverflowMetricItem = Pick<TopbarOverflowItem, "id" | "priority">;

type HiddenTopbarActionArgs = {
  items: TopbarOverflowMetricItem[];
  availableWidth: number;
  itemWidths: ReadonlyMap<string, number>;
  gap: number;
  overflowTriggerWidth: number;
  fallbackItemWidth?: number;
};

const DEFAULT_FALLBACK_ITEM_WIDTH = 88;
const DEFAULT_OVERFLOW_TRIGGER_WIDTH = 40;

function setsMatch(set: Set<string>, values: string[]) {
  if (set.size !== values.length) return false;
  return values.every((value) => set.has(value));
}

function itemWidth(
  item: TopbarOverflowMetricItem,
  widths: ReadonlyMap<string, number>,
  fallback: number,
) {
  return widths.get(item.id) ?? fallback;
}

function totalActionWidth({
  items,
  hiddenIds,
  itemWidths,
  gap,
  overflowTriggerWidth,
  fallbackItemWidth,
}: {
  items: TopbarOverflowMetricItem[];
  hiddenIds: Set<string>;
  itemWidths: ReadonlyMap<string, number>;
  gap: number;
  overflowTriggerWidth: number;
  fallbackItemWidth: number;
}) {
  const visibleItems = items.filter((item) => !hiddenIds.has(item.id));
  const visibleWidth = visibleItems.reduce(
    (total, item) => total + itemWidth(item, itemWidths, fallbackItemWidth),
    0,
  );
  const controlCount = visibleItems.length + (hiddenIds.size > 0 ? 1 : 0);
  const gapWidth = Math.max(0, controlCount - 1) * gap;

  return visibleWidth + (hiddenIds.size > 0 ? overflowTriggerWidth : 0) + gapWidth;
}

export function getHiddenTopbarActionIds({
  items,
  availableWidth,
  itemWidths,
  gap,
  overflowTriggerWidth,
  fallbackItemWidth = DEFAULT_FALLBACK_ITEM_WIDTH,
}: HiddenTopbarActionArgs): string[] {
  const hiddenIds = new Set<string>();
  const hideableItems = [...items].sort((a, b) => a.priority - b.priority);

  while (
    totalActionWidth({
      items,
      hiddenIds,
      itemWidths,
      gap,
      overflowTriggerWidth,
      fallbackItemWidth,
    }) > availableWidth
  ) {
    const nextItem = hideableItems.find((item) => !hiddenIds.has(item.id));
    if (!nextItem) break;
    hiddenIds.add(nextItem.id);
  }

  return items.filter((item) => hiddenIds.has(item.id)).map((item) => item.id);
}

function readFlexGap(element: HTMLElement) {
  const styles = window.getComputedStyle(element);
  const gap = Number.parseFloat(styles.columnGap || styles.gap || "0");
  return Number.isFinite(gap) ? gap : 0;
}

type TopbarActionOverflowProps = {
  items: TopbarOverflowItem[];
  className?: string;
};

function measureVisibleItems(
  itemRefs: Map<string, HTMLDivElement>,
  itemWidths: Map<string, number>,
) {
  for (const [id, element] of itemRefs) {
    const width = Math.ceil(element.getBoundingClientRect().width);
    if (width > 0) itemWidths.set(id, width);
  }
}

function useTopbarOverflowState(items: TopbarOverflowItem[]) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const overflowTriggerRef = useRef<HTMLButtonElement | null>(null);
  const itemRefs = useRef(new Map<string, HTMLDivElement>());
  const itemWidths = useRef(new Map<string, number>());
  const [hiddenIds, setHiddenIds] = useState<Set<string>>(() => new Set());
  const [measuredWidth, setMeasuredWidth] = useState(0);

  const registerItem = useCallback(
    (id: string) => (element: HTMLDivElement | null) => {
      if (element) {
        itemRefs.current.set(id, element);
      } else {
        itemRefs.current.delete(id);
      }
    },
    [],
  );

  useLayoutEffect(() => {
    const element = containerRef.current;
    if (!element) return;

    const updateWidth = () => setMeasuredWidth(element.clientWidth);
    updateWidth();

    const observer = new ResizeObserver(updateWidth);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  useLayoutEffect(() => {
    const container = containerRef.current;
    if (!container || container.clientWidth <= 0) return;

    measureVisibleItems(itemRefs.current, itemWidths.current);

    const overflowWidth =
      overflowTriggerRef.current?.getBoundingClientRect().width || DEFAULT_OVERFLOW_TRIGGER_WIDTH;
    const nextHiddenIds = getHiddenTopbarActionIds({
      items,
      availableWidth: container.clientWidth,
      itemWidths: itemWidths.current,
      gap: readFlexGap(container),
      overflowTriggerWidth: overflowWidth,
    });

    setHiddenIds((current) =>
      setsMatch(current, nextHiddenIds) ? current : new Set(nextHiddenIds),
    );
  }, [items, measuredWidth]);

  return { containerRef, overflowTriggerRef, registerItem, hiddenIds };
}

function OverflowTrigger({ triggerRef }: { triggerRef: Ref<HTMLButtonElement> }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <PopoverTrigger asChild>
          <Button
            ref={triggerRef}
            size="sm"
            variant="outline"
            className="h-8 cursor-pointer px-2"
            aria-label="More top bar actions"
            data-testid="topbar-action-overflow-trigger"
          >
            <IconDots className="h-4 w-4" />
          </Button>
        </PopoverTrigger>
      </TooltipTrigger>
      <TooltipContent>More actions</TooltipContent>
    </Tooltip>
  );
}

function OverflowPopover({
  items,
  triggerRef,
}: {
  items: TopbarOverflowItem[];
  triggerRef: Ref<HTMLButtonElement>;
}) {
  if (items.length === 0) return null;

  return (
    <Popover>
      <OverflowTrigger triggerRef={triggerRef} />
      <PopoverContent
        align="end"
        className="w-auto max-w-[min(520px,calc(100vw-1rem))] gap-0 rounded-md p-1.5"
      >
        <div className="flex max-w-[480px] flex-wrap items-center justify-end gap-2">
          {items.map((item) => (
            <div
              key={item.id}
              aria-label={item.label}
              className="inline-flex shrink-0 items-center [&_button]:h-8 [&_button]:text-xs"
            >
              {item.content}
            </div>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}

export function TopbarActionOverflow({ items, className }: TopbarActionOverflowProps) {
  const { containerRef, overflowTriggerRef, registerItem, hiddenIds } =
    useTopbarOverflowState(items);
  const hiddenItems = items.filter((item) => hiddenIds.has(item.id));

  return (
    <div
      ref={containerRef}
      className={cn(
        "flex min-w-0 w-full items-center justify-end gap-2 overflow-hidden",
        className,
      )}
      data-testid="topbar-action-overflow"
    >
      {items.map((item) =>
        hiddenIds.has(item.id) ? null : (
          <div key={item.id} ref={registerItem(item.id)} className="shrink-0">
            {item.content}
          </div>
        ),
      )}
      <OverflowPopover items={hiddenItems} triggerRef={overflowTriggerRef} />
    </div>
  );
}
