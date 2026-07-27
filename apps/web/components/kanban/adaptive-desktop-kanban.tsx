"use client";

import { Children, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Badge } from "@kandev/ui/badge";
import type { WorkflowStep } from "@/components/kanban-column";
import { formatWipCount, isOverWipLimit } from "@/lib/kanban/wip-limit";
import { cn } from "@/lib/utils";
import {
  getKanbanColumnGridTemplate,
  getLeadingKanbanColumnIndex,
  shouldUseWindowedKanban,
} from "./kanban-grid-template";

type AdaptiveDesktopKanbanProps = {
  steps: WorkflowStep[];
  taskCounts: Record<string, number>;
  activeIndex: number;
  onActiveIndexChange: (index: number) => void;
  children: ReactNode;
};

function DesktopStageNavigator({
  steps,
  taskCounts,
  activeIndex,
  onSelect,
}: Omit<AdaptiveDesktopKanbanProps, "children" | "onActiveIndexChange"> & {
  onSelect: (index: number) => void;
}) {
  return (
    <nav
      aria-label="Workflow stages"
      className="min-w-40 border-r border-border/70 bg-muted/20 p-2"
      data-testid="desktop-kanban-stage-navigator"
    >
      <div className="mb-2 px-2 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
        Stages
      </div>
      <div className="space-y-1">
        {steps.map((step, index) => {
          const count = taskCounts[step.id] ?? 0;
          const label = formatWipCount(count, step.wip_limit);
          const active = index === activeIndex;
          const overWipLimit = isOverWipLimit(count, step.wip_limit);
          return (
            <button
              key={step.id}
              type="button"
              data-testid={`desktop-kanban-stage-${step.id}`}
              data-active={active}
              aria-current={active ? "step" : undefined}
              onClick={() => onSelect(index)}
              className={cn(
                "flex min-h-11 w-full cursor-pointer items-center gap-2 rounded-md px-2 text-left text-sm transition-colors",
                active ? "bg-background shadow-sm" : "hover:bg-muted/70",
              )}
            >
              <span
                className="h-2 w-2 shrink-0 rounded-full"
                style={{ backgroundColor: step.color }}
                aria-hidden
              />
              <span className="min-w-0 flex-1 truncate font-medium">{step.title}</span>
              <Badge
                variant="secondary"
                className={cn(
                  "h-5 shrink-0 px-1.5 text-xs tabular-nums",
                  overWipLimit &&
                    "border-amber-500/50 bg-amber-500/15 text-amber-700 dark:text-amber-300",
                )}
              >
                {label}
              </Badge>
            </button>
          );
        })}
      </div>
    </nav>
  );
}

export function AdaptiveDesktopKanban({
  steps,
  taskCounts,
  activeIndex,
  onActiveIndexChange,
  children,
}: AdaptiveDesktopKanbanProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const scrollWindowRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(0);
  const columns = useMemo(() => Children.toArray(children), [children]);
  const windowed = shouldUseWindowedKanban(containerWidth, steps.length);

  useEffect(() => {
    const element = rootRef.current;
    if (!element) return;
    const updateWidth = () => setContainerWidth(element.clientWidth);
    updateWidth();
    const observer = new ResizeObserver(updateWidth);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const selectStage = useCallback(
    (index: number) => {
      onActiveIndexChange(index);
      const stepId = steps[index]?.id;
      const column = stepId
        ? (scrollWindowRef.current?.querySelector(
            `[data-kanban-step-id="${stepId}"]`,
          ) as HTMLElement | null)
        : null;
      column?.scrollIntoView({ block: "nearest", inline: "start" });
    },
    [onActiveIndexChange],
  );

  const syncSelection = useCallback(() => {
    const window = scrollWindowRef.current;
    if (!window) return;
    const laneGrid = window.firstElementChild;
    const offsets = laneGrid
      ? Array.from(laneGrid.children, (column) => (column as HTMLElement).offsetLeft)
      : [];
    const nextIndex = getLeadingKanbanColumnIndex(window.scrollLeft, offsets);
    if (nextIndex !== activeIndex) onActiveIndexChange(nextIndex);
  }, [activeIndex, onActiveIndexChange]);

  const laneGrid = (
    <div
      data-testid="desktop-kanban-lane-grid"
      className="grid h-full min-w-full gap-0"
      style={{ gridTemplateColumns: getKanbanColumnGridTemplate(steps.length) }}
    >
      {columns.map((column, index) => (
        <div
          key={steps[index]?.id ?? index}
          data-kanban-step-id={steps[index]?.id}
          className="min-w-0 snap-start"
        >
          {column}
        </div>
      ))}
    </div>
  );

  return (
    <div ref={rootRef} data-testid="desktop-kanban-layout" className="h-full min-w-0">
      {windowed ? (
        <div className="grid h-full min-w-0 grid-cols-[10rem_minmax(0,1fr)]">
          <DesktopStageNavigator
            steps={steps}
            taskCounts={taskCounts}
            activeIndex={activeIndex}
            onSelect={selectStage}
          />
          <div
            ref={scrollWindowRef}
            data-testid="desktop-kanban-scroll-window"
            className="min-w-0 overflow-x-auto snap-x snap-mandatory"
            onScroll={syncSelection}
          >
            {laneGrid}
          </div>
        </div>
      ) : (
        laneGrid
      )}
    </div>
  );
}
