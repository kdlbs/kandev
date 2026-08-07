"use client";

import { useEffect, useRef, useState } from "react";

/**
 * Holds a transient "busy" flag on for at least `minMs` once it turns on.
 *
 * A refresh that resolves from cache can settle in well under a frame, which
 * makes a spinner appear and vanish as a flicker — strictly worse than showing
 * nothing, because the user registers movement without reading it. Stretching
 * the tail keeps the indicator legible; it never delays the data itself, only
 * the disappearance of the indicator.
 *
 * Turning on is always immediate. Re-activating while the tail is still running
 * restarts the window from that moment.
 */
export function useMinVisibleDuration(active: boolean, minMs: number): boolean {
  const [visible, setVisible] = useState(active);
  const shownAtRef = useRef<number | null>(active ? Date.now() : null);

  useEffect(() => {
    if (active) {
      shownAtRef.current = Date.now();
      setVisible(true);
      return;
    }
    if (!visible) return;
    const remaining = minMs - (Date.now() - (shownAtRef.current ?? 0));
    if (remaining <= 0) {
      setVisible(false);
      return;
    }
    const timer = setTimeout(() => setVisible(false), remaining);
    return () => clearTimeout(timer);
  }, [active, visible, minMs]);

  return visible;
}
