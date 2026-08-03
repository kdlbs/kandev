"use client";

import { useCallback, useEffect, useState } from "react";

export const RAIL_MIN_WIDTH = 200;
export const RAIL_DEFAULT_WIDTH = 288;
/** Share of the window the rail may take. The transcript is the point of the
 *  page, so the switcher beside it never gets to dominate. */
const RAIL_MAX_FRACTION = 0.4;
const STORAGE_KEY = "kandev.runsRailWidth";

function clamp(width: number, maxWidth: number): number {
  return Math.min(maxWidth, Math.max(RAIL_MIN_WIDTH, width));
}

function readStored(): number {
  if (typeof window === "undefined") return RAIL_DEFAULT_WIDTH;
  const raw = window.localStorage.getItem(STORAGE_KEY);
  const parsed = raw ? Number.parseInt(raw, 10) : Number.NaN;
  return Number.isFinite(parsed) ? parsed : RAIL_DEFAULT_WIDTH;
}

/**
 * A draggable width for the runs rail, remembered across sessions.
 *
 * Run rows carry timestamps and status words that wrap awkwardly in a narrow
 * column, and how much room the reader wants depends on whether they are
 * scanning the switcher or reading the transcript beside it. The sidebar is
 * resizable for the same reason; this mirrors its drag so both edges behave the
 * same way under the mouse.
 *
 * Local rather than in the app store: it is a per-device viewing preference, in
 * the same family as the sidebar width, and nothing outside this page reads it.
 */
export function useRailWidth() {
  // Starts at the default and adopts the stored value after mount, so the
  // server and first client render agree.
  const [width, setWidth] = useState(RAIL_DEFAULT_WIDTH);
  const [resizing, setResizing] = useState(false);

  useEffect(() => {
    setWidth((current) => {
      const stored = readStored();
      return stored === current ? current : clamp(stored, window.innerWidth * RAIL_MAX_FRACTION);
    });
  }, []);

  const onResizeStart = useCallback((event: React.MouseEvent) => {
    event.preventDefault();
    setResizing(true);
    const startX = event.clientX;
    const startWidth = readStored();
    const maxWidth = Math.floor(window.innerWidth * RAIL_MAX_FRACTION);

    const onMove = (moveEvent: MouseEvent) => {
      // The rail is on the right, so dragging left widens it — the delta is
      // inverted relative to the sidebar's handle.
      setWidth(clamp(startWidth + (startX - moveEvent.clientX), maxWidth));
    };
    const onUp = () => {
      setResizing(false);
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
  }, []);

  useEffect(() => {
    if (resizing) return;
    window.localStorage.setItem(STORAGE_KEY, String(width));
  }, [resizing, width]);

  return { width, resizing, onResizeStart };
}
