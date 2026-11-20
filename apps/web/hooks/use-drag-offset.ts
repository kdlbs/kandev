"use client";

import { useCallback, useRef, useState } from "react";

/**
 * A transparent layer over the whole viewport for the length of a drag, so
 * `mousemove` keeps reaching this document when the pointer crosses an iframe.
 */
function createDragShield() {
  const shield = document.createElement("div");
  shield.setAttribute("data-selection-comment-drag-shield", "");
  shield.style.cssText = "position:fixed;inset:0;z-index:2147483647;cursor:grabbing;";
  document.body.appendChild(shield);
  return shield;
}

type DragStart = { clientX: number; clientY: number; preventDefault: () => void };

/**
 * Follows one mouse drag from a press, reporting the distance moved since it.
 * The shield receives the release, so the pressed element gets no click or
 * double-click; read `event.detail` on the press to detect a double-click.
 */
export function startMouseDrag(
  start: DragStart,
  handlers: { onMove: (dx: number, dy: number) => void; onEnd?: (dx: number, dy: number) => void },
) {
  start.preventDefault();
  const shield = createDragShield();
  const delta = (event: MouseEvent) =>
    [event.clientX - start.clientX, event.clientY - start.clientY] as const;
  const onMouseMove = (event: MouseEvent) => handlers.onMove(...delta(event));
  const onMouseUp = (event: MouseEvent) => {
    shield.remove();
    document.removeEventListener("mousemove", onMouseMove);
    document.removeEventListener("mouseup", onMouseUp);
    handlers.onEnd?.(...delta(event));
  };
  document.addEventListener("mousemove", onMouseMove);
  document.addEventListener("mouseup", onMouseUp);
}

/** An offset that accumulates across drags, such as for a draggable popover. */
export function useDrag() {
  const [offset, setOffset] = useState({ dx: 0, dy: 0 });
  const offsetRef = useRef(offset);
  offsetRef.current = offset;

  const onMouseDown = useCallback((event: React.MouseEvent) => {
    const origin = offsetRef.current;
    startMouseDrag(event, {
      onMove: (dx, dy) => setOffset({ dx: origin.dx + dx, dy: origin.dy + dy }),
    });
  }, []);

  return { offset, onMouseDown };
}
