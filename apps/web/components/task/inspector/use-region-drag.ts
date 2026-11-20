"use client";

import { useCallback, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import type { RegionHandle } from "@/components/shared/region-annotation/region-overlay";
import type { PreviewRegionView } from "@/hooks/use-preview-capture";
import {
  moveRegion,
  resizeRegionFromCorner,
  type RegionBounds,
  type RegionRect,
} from "@/lib/regions/geometry";

const CLICK_SLOP_PX = 5;
const MIN_REGION_PX = 8;

type Override = { rect: RegionRect; dragId: number };

type Options = {
  regions: PreviewRegionView[];
  bounds: () => RegionBounds | null;
  onOpen: (id: string) => void;
  onRecapture: (id: string, rect: RegionRect) => Promise<boolean>;
};

function draggedRect(
  start: RegionRect,
  handle: RegionHandle,
  delta: { x: number; y: number },
  bounds: RegionBounds,
) {
  if (handle === "move") return moveRegion(start, delta.x, delta.y, bounds);
  return resizeRegionFromCorner(start, handle, delta, bounds, MIN_REGION_PX);
}

function withoutKey(current: Record<string, Override>, id: string) {
  const next = { ...current };
  delete next[id];
  return next;
}

/**
 * Moves and resizes region outlines in the parent layer. A press without
 * movement opens the region; a release after a drag recaptures it, and a
 * failed recapture returns the outline to where it was.
 */
export function useRegionDrag({ regions, bounds, onOpen, onRecapture }: Options) {
  const [overrides, setOverrides] = useState<Record<string, Override>>({});
  const nextDragId = useRef(0);

  const onHandlePointerDown = useCallback(
    (index: number, handle: RegionHandle, event: ReactPointerEvent<HTMLElement>) => {
      const region = regions.find((candidate) => candidate.index === index);
      const area = bounds();
      if (!region || !area) return;
      event.preventDefault();
      const target = event.currentTarget;
      const pointerId = event.pointerId;
      const dragId = nextDragId.current++;
      const origin = { x: event.clientX, y: event.clientY };
      let latest = region.rect;
      let moved = false;
      target.setPointerCapture?.(pointerId);

      const onMove = (move: PointerEvent) => {
        if (move.pointerId !== pointerId) return;
        const delta = { x: move.clientX - origin.x, y: move.clientY - origin.y };
        if (!moved && Math.hypot(delta.x, delta.y) <= CLICK_SLOP_PX) return;
        moved = true;
        latest = draggedRect(region.rect, handle, delta, area);
        setOverrides((current) => ({ ...current, [region.id]: { rect: latest, dragId } }));
      };
      const clearOverride = () =>
        setOverrides((current) =>
          current[region.id]?.dragId === dragId ? withoutKey(current, region.id) : current,
        );
      const finish = (end: PointerEvent) => {
        if (end.pointerId !== pointerId) return;
        target.removeEventListener("pointermove", onMove);
        target.removeEventListener("pointerup", finish);
        target.removeEventListener("pointercancel", finish);
        if (end.type === "pointercancel" || !moved) {
          clearOverride();
          if (end.type === "pointerup" && handle === "move") onOpen(region.id);
          return;
        }
        const rect = latest;
        try {
          void onRecapture(region.id, rect).then(clearOverride, clearOverride);
        } catch {
          clearOverride();
        }
      };
      target.addEventListener("pointermove", onMove);
      target.addEventListener("pointerup", finish);
      target.addEventListener("pointercancel", finish);
    },
    [bounds, onOpen, onRecapture, regions],
  );

  const displayed = regions.map((region) => {
    const override = overrides[region.id];
    // A region being moved or resized is drawn whole, without its container clip.
    return override ? { ...region, rect: override.rect, clip: undefined } : region;
  });
  return { regions: displayed, onHandlePointerDown };
}
