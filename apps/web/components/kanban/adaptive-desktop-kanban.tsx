"use client";

import { useRef, useState, type MouseEvent, type ReactNode } from "react";
import type { WorkflowStep } from "@/components/kanban-column";
import { getKanbanColumnGridTemplate, KANBAN_COLUMN_MIN_PX } from "./kanban-grid-template";

type AdaptiveDesktopKanbanProps = {
  steps: WorkflowStep[];
  isDragging?: boolean;
  renderColumn: (step: WorkflowStep) => ReactNode;
};

export const KANBAN_DRAG_END_PADDING = `max(0px, calc(100% - ${KANBAN_COLUMN_MIN_PX}px))`;

const PAN_ACTIVATION_DISTANCE_PX = 4;
const INTERACTIVE_TARGET_SELECTOR = [
  "a[href]",
  "button",
  "input",
  "select",
  "textarea",
  "label",
  "summary",
  "[contenteditable]",
  "[draggable='true']",
  "[data-kanban-card]",
  "[role='button'], [role='link'], [role='checkbox'], [role='radio'], [role='menuitem'], [role='option'], [role='switch'], [role='tab'], [role='combobox'], [role='textbox'], [role='gridcell'], [role='treeitem']",
  "[tabindex]:not([tabindex='-1'])",
].join(", ");

type PanStart = {
  clientX: number;
  scrollLeft: number;
};

export function AdaptiveDesktopKanban({
  steps,
  isDragging = false,
  renderColumn,
}: AdaptiveDesktopKanbanProps) {
  const scrollWindowRef = useRef<HTMLDivElement | null>(null);
  const panStartRef = useRef<PanStart | null>(null);
  const [isPanCandidate, setIsPanCandidate] = useState(false);
  const [isPanning, setIsPanning] = useState(false);

  const cancelPan = () => {
    panStartRef.current = null;
    scrollWindowRef.current?.style.removeProperty("scroll-snap-type");
    setIsPanCandidate(false);
    setIsPanning(false);
  };

  const handleMouseDown = (event: MouseEvent<HTMLDivElement>) => {
    if (event.button !== 0 || isInteractiveTarget(event.target, event.currentTarget)) return;

    panStartRef.current = {
      clientX: event.clientX,
      scrollLeft: event.currentTarget.scrollLeft,
    };
    setIsPanCandidate(true);
  };

  const handleMouseMove = (event: MouseEvent<HTMLDivElement>) => {
    const panStart = panStartRef.current;
    if (!panStart) return;
    if ((event.buttons & 1) === 0) {
      cancelPan();
      return;
    }

    const delta = panStart.clientX - event.clientX;
    if (!isPanning && Math.abs(delta) <= PAN_ACTIVATION_DISTANCE_PX) return;

    if (!isPanning) {
      window.getSelection()?.removeAllRanges();
      event.currentTarget.style.scrollSnapType = "none";
      setIsPanning(true);
    }
    event.preventDefault();
    event.currentTarget.scrollLeft = panStart.scrollLeft + delta;
  };

  return (
    <div data-testid="desktop-kanban-layout" className="h-full min-h-0 min-w-0">
      <div
        ref={scrollWindowRef}
        data-testid="desktop-kanban-scroll-window"
        className={`h-full min-h-0 min-w-0 overflow-x-auto snap-x snap-mandatory ${
          isPanCandidate ? "cursor-grabbing" : ""
        } ${isPanning ? "select-none" : ""}`}
        style={{ scrollSnapType: isPanning ? "none" : undefined }}
        onMouseDown={handleMouseDown}
        onMouseMove={handleMouseMove}
        onMouseUp={cancelPan}
        onMouseLeave={cancelPan}
      >
        <div
          data-testid="desktop-kanban-lane-grid"
          className="grid h-full min-h-0 min-w-full gap-0"
          style={{
            gridTemplateColumns: getKanbanColumnGridTemplate(steps.length),
            paddingInlineEnd: isDragging ? KANBAN_DRAG_END_PADDING : undefined,
          }}
        >
          {steps.map((step) => (
            <div key={step.id} data-kanban-step-id={step.id} className="min-h-0 min-w-0 snap-start">
              {renderColumn(step)}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function isInteractiveTarget(target: EventTarget | null, boundary: HTMLElement): boolean {
  if (!(target instanceof Element)) return true;

  for (
    let element: Element | null = target;
    element && element !== boundary;
    element = element.parentElement
  ) {
    if (element.matches(INTERACTIVE_TARGET_SELECTOR)) return true;
  }
  return false;
}
