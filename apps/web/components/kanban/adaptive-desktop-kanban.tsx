"use client";

import { useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { WorkflowStep } from "@/components/kanban-column";
import { useDesktopKanbanPan } from "@/hooks/domains/kanban/use-desktop-kanban-pan";
import { useKanbanOverflow } from "@/hooks/domains/kanban/use-kanban-overflow";
import { getKanbanColumnGridTemplate, KANBAN_COLUMN_MIN_PX } from "./kanban-grid-template";
import { KanbanOverflowFades } from "./kanban-overflow-fades";

type AdaptiveDesktopKanbanProps = {
  columnHeight?: string;
  steps: WorkflowStep[];
  isDragging?: boolean;
  renderColumn: (step: WorkflowStep) => ReactNode;
};

const KANBAN_DRAG_END_RESERVE = `max(0px, calc(100cqw - ${KANBAN_COLUMN_MIN_PX}px))`;

export function AdaptiveDesktopKanban({
  columnHeight,
  steps,
  isDragging = false,
  renderColumn,
}: AdaptiveDesktopKanbanProps) {
  const scrollWindowRef = useRef<HTMLDivElement | null>(null);
  const laneGridRef = useRef<HTMLDivElement | null>(null);
  const { t } = useTranslation();
  const { cancelPan, handleMouseDown, handleMouseMove, isPanCandidate, isPanning } =
    useDesktopKanbanPan(scrollWindowRef);
  const overflow = useKanbanOverflow(scrollWindowRef, {
    axis: "horizontal",
    contentRef: laneGridRef,
    revision: `${steps.length}:${isDragging ? 1 : 0}`,
  });

  return (
    <div
      data-testid="desktop-kanban-layout"
      className="relative h-full min-h-0 min-w-0"
      style={{ height: columnHeight ? "auto" : undefined }}
    >
      <div
        ref={scrollWindowRef}
        aria-label={t("kanban:columns")}
        data-testid="desktop-kanban-scroll-window"
        className={`kanban-scroll-region h-full min-h-0 min-w-0 overflow-x-auto overscroll-y-auto snap-x snap-mandatory ${
          isPanCandidate ? "cursor-grabbing" : ""
        } ${isPanning ? "select-none" : ""} ${isDragging ? "scrollbar-hide" : ""}`}
        data-kanban-scroll-axis="horizontal"
        data-kanban-scroll-active={overflow.isScrolling}
        data-kanban-scroll-left={overflow.canScrollLeft}
        data-kanban-scroll-right={overflow.canScrollRight}
        style={{
          height: columnHeight ? "auto" : undefined,
          containerType: "inline-size",
          scrollSnapType: isPanning ? "none" : undefined,
        }}
        onMouseDown={handleMouseDown}
        onMouseMove={handleMouseMove}
        onMouseUp={cancelPan}
        onMouseLeave={cancelPan}
        tabIndex={0}
      >
        <div
          className="flex h-full min-h-0"
          style={{
            height: columnHeight,
            width: `calc(max(100cqw, ${steps.length * KANBAN_COLUMN_MIN_PX}px) + ${
              isDragging ? KANBAN_DRAG_END_RESERVE : "0px"
            })`,
          }}
        >
          <div
            ref={laneGridRef}
            data-testid="desktop-kanban-lane-grid"
            className="grid h-full min-h-0 flex-none gap-0"
            style={{
              gridTemplateColumns: getKanbanColumnGridTemplate(steps.length),
              width: `max(100cqw, ${steps.length * KANBAN_COLUMN_MIN_PX}px)`,
            }}
          >
            {steps.map((step) => (
              <div
                key={step.id}
                data-kanban-step-id={step.id}
                className="min-h-0 min-w-0 snap-start"
              >
                {renderColumn(step)}
              </div>
            ))}
          </div>
          {isDragging && <DragEndReserve />}
        </div>
      </div>
      <KanbanOverflowFades axis="horizontal" state={overflow} />
    </div>
  );
}

function DragEndReserve() {
  return (
    <div
      data-testid="desktop-kanban-drag-end-reserve"
      aria-hidden="true"
      className="h-full flex-none"
      style={{ width: KANBAN_DRAG_END_RESERVE }}
    />
  );
}
