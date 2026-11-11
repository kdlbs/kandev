import { useCallback, useEffect, useRef, useState } from "react";

export const HOLD_DELAY_MS = 300;
const HOLD_SLOP_PX = 6;

/**
 * Press and hold the stage (or hold Space) to show A while held. A press that
 * moves is a pan, not a hold. Releasing returns to the current comparison.
 */
export function useHoldToCompare(enabled: boolean) {
  const [holding, setHolding] = useState(false);
  const press = useRef<{ id: number; x: number; y: number } | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearTimer = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
  }, []);
  const release = useCallback(() => {
    clearTimer();
    press.current = null;
    setHolding(false);
  }, [clearTimer]);

  useEffect(() => {
    if (!enabled) release();
  }, [enabled, release]);
  useEffect(() => clearTimer, [clearTimer]);
  // Space can be released after focus moves off the stage, and the window can
  // lose focus mid-hold; either ends the hold.
  useEffect(() => {
    if (!holding) return;
    const onKeyUp = (event: KeyboardEvent) => {
      if (event.key === " ") release();
    };
    window.addEventListener("keyup", onKeyUp);
    window.addEventListener("blur", release);
    return () => {
      window.removeEventListener("keyup", onKeyUp);
      window.removeEventListener("blur", release);
    };
  }, [holding, release]);

  const onPointerDown = useCallback(
    (event: React.PointerEvent<HTMLElement>) => {
      if (!enabled || !event.isPrimary) return release();
      press.current = { id: event.pointerId, x: event.clientX, y: event.clientY };
      clearTimer();
      timer.current = setTimeout(() => setHolding(true), HOLD_DELAY_MS);
    },
    [clearTimer, enabled, release],
  );
  const onPointerMove = useCallback(
    (event: React.PointerEvent<HTMLElement>) => {
      const origin = press.current;
      if (!origin || origin.id !== event.pointerId) return;
      const moved = Math.hypot(event.clientX - origin.x, event.clientY - origin.y);
      if (moved > HOLD_SLOP_PX) release();
    },
    [release],
  );

  const keyDown = useCallback(
    (event: { key: string; repeat: boolean }) => {
      if (!enabled || event.key !== " ") return false;
      if (!event.repeat) setHolding(true);
      return true;
    },
    [enabled],
  );
  const keyUp = useCallback(
    (event: { key: string }) => {
      if (!enabled || event.key !== " ") return false;
      release();
      return true;
    },
    [enabled, release],
  );

  const onPointerEnd = useCallback(
    (_event?: React.PointerEvent<HTMLElement>) => release(),
    [release],
  );

  return {
    holding,
    keyDown,
    keyUp,
    pointer: {
      onPointerDown,
      onPointerMove,
      onPointerUp: onPointerEnd,
      onPointerCancel: onPointerEnd,
    },
  };
}
