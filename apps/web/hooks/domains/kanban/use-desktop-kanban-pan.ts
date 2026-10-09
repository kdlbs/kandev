import { useCallback, useRef, useState, type MouseEvent, type RefObject } from "react";

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
  "[tabindex]:not([tabindex='-1']):not(.kanban-scroll-region)",
].join(", ");

type PanStart = {
  clientX: number;
  scrollLeft: number;
};

export function useDesktopKanbanPan(scrollWindowRef: RefObject<HTMLDivElement | null>) {
  const panStartRef = useRef<PanStart | null>(null);
  const [isPanCandidate, setIsPanCandidate] = useState(false);
  const [isPanning, setIsPanning] = useState(false);

  const cancelPan = useCallback(() => {
    panStartRef.current = null;
    scrollWindowRef.current?.style.removeProperty("scroll-snap-type");
    setIsPanCandidate(false);
    setIsPanning(false);
  }, [scrollWindowRef]);

  const handleMouseDown = useCallback((event: MouseEvent<HTMLDivElement>) => {
    if (event.button !== 0 || isInteractiveTarget(event.target, event.currentTarget)) return;

    panStartRef.current = {
      clientX: event.clientX,
      scrollLeft: event.currentTarget.scrollLeft,
    };
    setIsPanCandidate(true);
  }, []);

  const handleMouseMove = useCallback(
    (event: MouseEvent<HTMLDivElement>) => {
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
    },
    [cancelPan, isPanning],
  );

  return { cancelPan, handleMouseDown, handleMouseMove, isPanCandidate, isPanning };
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
