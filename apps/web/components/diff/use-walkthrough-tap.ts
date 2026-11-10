import { useEffect, useRef, type PointerEvent } from "react";

type TouchTap = {
  pointerId: number;
  x: number;
  y: number;
  time: number;
};

/** Owner-scoped so the click guard survives an activated control's removal. */
export function useWalkthroughTap() {
  const pending = useRef<TouchTap | null>(null);
  const removeGuard = useRef<(() => void) | null>(null);
  useEffect(() => () => removeGuard.current?.(), []);

  function guardCompatibilityClick(owner: Document) {
    removeGuard.current?.();
    const clear = () => {
      pending.current = null;
      owner.removeEventListener("click", consume, true);
      owner.removeEventListener("pointerdown", clear, true);
      removeGuard.current = null;
    };
    const consume = (event: MouseEvent) => {
      if (event.detail === 0) return;
      event.preventDefault();
      event.stopImmediatePropagation();
    };
    owner.addEventListener("click", consume, true);
    owner.addEventListener("pointerdown", clear, true);
    removeGuard.current = clear;
  }

  return (onTap: () => void) => ({
    onPointerDown(event: PointerEvent<HTMLElement>) {
      if (event.pointerType === "touch") guardCompatibilityClick(event.currentTarget.ownerDocument);
      pending.current =
        event.pointerType === "touch" && event.isPrimary && event.button === 0
          ? {
              pointerId: event.pointerId,
              x: event.clientX,
              y: event.clientY,
              time: event.timeStamp,
            }
          : null;
    },
    onPointerMoveCapture(event: PointerEvent<HTMLElement>) {
      const tap = pending.current;
      if (tap && Math.hypot(event.clientX - tap.x, event.clientY - tap.y) > 8) {
        pending.current = null;
      }
    },
    onPointerCancel() {
      pending.current = null;
    },
    onContextMenu() {
      pending.current = null;
    },
    onPointerUp(event: PointerEvent<HTMLElement>) {
      const tap = pending.current;
      pending.current = null;
      if (!tap || event.pointerId !== tap.pointerId || event.timeStamp - tap.time >= 500) return;
      if (Math.hypot(event.clientX - tap.x, event.clientY - tap.y) > 8) return;
      const rect = event.currentTarget.getBoundingClientRect();
      if (
        event.clientX < rect.left ||
        event.clientX > rect.right ||
        event.clientY < rect.top ||
        event.clientY > rect.bottom
      )
        return;
      onTap();
    },
  });
}
