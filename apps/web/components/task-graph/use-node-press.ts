import { useCallback, useEffect, useRef, type PointerEvent } from "react";

const LONG_PRESS_MS = 500;
const MOVE_TOLERANCE_PX = 8;

/**
 * Tap activates; a touch long press emphasizes instead and swallows the click
 * that follows. Moving cancels, so a drag still pans the canvas.
 */
export function useNodePress(onActivate: () => void, onLongPress: () => void) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const origin = useRef<{ x: number; y: number } | null>(null);
  const longPressed = useRef(false);

  const cancel = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
    origin.current = null;
  }, []);

  useEffect(() => cancel, [cancel]);

  const onPointerDown = useCallback(
    (event: PointerEvent) => {
      longPressed.current = false;
      if (event.pointerType !== "touch") return;
      origin.current = { x: event.clientX, y: event.clientY };
      timer.current = setTimeout(() => {
        longPressed.current = true;
        timer.current = null;
        onLongPress();
      }, LONG_PRESS_MS);
    },
    [onLongPress],
  );

  const onPointerMove = useCallback(
    (event: PointerEvent) => {
      const start = origin.current;
      if (!start) return;
      if (Math.hypot(event.clientX - start.x, event.clientY - start.y) > MOVE_TOLERANCE_PX)
        cancel();
    },
    [cancel],
  );

  const onClick = useCallback(() => {
    if (longPressed.current) {
      longPressed.current = false;
      return;
    }
    onActivate();
  }, [onActivate]);

  return { onPointerDown, onPointerMove, onPointerUp: cancel, onPointerCancel: cancel, onClick };
}
