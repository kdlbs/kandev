"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { toggleGroupDisclosure, type DisclosureOverrides } from "@/lib/kanban/steps-disclosure";

/**
 * Owns one Display surface's disclosure override map. The map is ephemeral —
 * scoped to a single visit to a single surface — and is reset on exactly two
 * events: the surface closing, and (while it stays open) `surfaceKey`
 * changing, which is how a breakpoint crossing that hands the Steps section
 * from one surface to the other is observed without relying on React
 * unmount-on-close (the drawer is not guaranteed to unmount on close).
 */
export function useStepsDisclosureOverrides(open: boolean, surfaceKey: string) {
  const [overrides, setOverrides] = useState<DisclosureOverrides>({});
  const prevOpenRef = useRef(open);
  const prevSurfaceKeyRef = useRef(surfaceKey);

  useEffect(() => {
    const closed = prevOpenRef.current && !open;
    const crossedWhileOpen =
      open && prevOpenRef.current && surfaceKey !== prevSurfaceKeyRef.current;
    if (closed || crossedWhileOpen) {
      setOverrides({});
    }
    prevOpenRef.current = open;
    prevSurfaceKeyRef.current = surfaceKey;
  }, [open, surfaceKey]);

  const toggleDisclosure = useCallback((workflowId: string, defaultValue: boolean) => {
    setOverrides((prev) => toggleGroupDisclosure(workflowId, prev, defaultValue));
  }, []);

  return { overrides, toggleDisclosure };
}
