"use client";

import { useEffect, useState } from "react";

/**
 * Keeps a row's unread dot visible for `delayMs` after the row becomes the
 * open task, so the user sees which task they opened before the dot clears.
 * The linger starts only when the row is selected while its dot is showing,
 * ends early when the row is deselected, and is never started by a message
 * that arrives while the task is already open. A zero delay disables it.
 */
export function useUnreadDotLinger(
  isSelected: boolean,
  dotShowing: boolean,
  delayMs: number,
): boolean {
  const [wasSelected, setWasSelected] = useState(false);
  const [lingering, setLingering] = useState(false);

  if (isSelected !== wasSelected) {
    setWasSelected(isSelected);
    setLingering(isSelected && dotShowing && delayMs > 0);
  }

  useEffect(() => {
    if (!lingering) return;
    const timer = setTimeout(() => setLingering(false), delayMs);
    return () => clearTimeout(timer);
  }, [lingering, delayMs]);

  return lingering;
}
