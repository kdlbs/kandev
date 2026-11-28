import { useCallback, useEffect, useMemo, useRef, useState } from "react";

/** How long the names stay up after the last interaction, so they can be read. */
const NAMES_LINGER_MS = 1200;

/**
 * When the Wipe handle shows its names: while any source holds them (the
 * handle, the divider slider under the pointer) and briefly after the split
 * moves on the image (`pulse`).
 */
export type WipeNamesReveal = {
  shown: boolean;
  hold: (source: string, on: boolean) => void;
  pulse: () => void;
  /** Drops every hold and the linger, for when the handle is no longer shown. */
  clear: () => void;
};

export function useWipeNamesReveal(): WipeNamesReveal {
  const [held, setHeld] = useState<ReadonlySet<string>>(() => new Set());
  const heldRef = useRef(held);
  heldRef.current = held;
  const [lingering, setLingering] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const pulse = useCallback(() => {
    setLingering(true);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setLingering(false), NAMES_LINGER_MS);
  }, []);
  const hold = useCallback(
    (source: string, on: boolean) => {
      const wasHeld = heldRef.current.has(source);
      if (wasHeld === on) return;
      const next = new Set(heldRef.current);
      if (on) next.add(source);
      else next.delete(source);
      heldRef.current = next;
      setHeld(next);
      if (!on && wasHeld) pulse();
    },
    [pulse],
  );
  const clear = useCallback(() => {
    clearTimeout(timer.current);
    heldRef.current = new Set();
    setHeld((current) => (current.size === 0 ? current : new Set()));
    setLingering(false);
  }, []);
  const shown = held.size > 0 || lingering;
  return useMemo(() => ({ shown, hold, pulse, clear }), [shown, hold, pulse, clear]);
}

/**
 * Pulses the names whenever the split moves on the image: the divider value
 * changes, or the image pans or zooms beneath it. Entering Wipe does not, and
 * leaving it clears the names, since a control that held them may be gone.
 */
export function usePulseOnSplitMove(reveal: WipeNamesReveal, active: boolean, key: string) {
  const last = useRef<string | null>(null);
  const { pulse, clear } = reveal;
  useEffect(() => {
    if (!active) {
      last.current = null;
      clear();
      return;
    }
    if (last.current !== null && last.current !== key) pulse();
    last.current = key;
  }, [active, key, pulse, clear]);
}
