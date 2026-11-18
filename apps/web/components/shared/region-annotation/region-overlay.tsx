"use client";

import type { PointerEvent as ReactPointerEvent } from "react";
import type { RegionCorner, RegionRect } from "@/lib/regions/geometry";
import { cn } from "@/lib/utils";

export type RegionHandle = "move" | RegionCorner;

export type RegionOverlayItem = {
  /** Position in the host's full region list; shown as `index + 1`. */
  index: number;
  rect: RegionRect;
  title?: string;
};

type Props = {
  items: readonly RegionOverlayItem[];
  selectedIndex: number | null;
  draft: RegionRect | null;
  /** Zoom of the host layer; lines and labels keep a constant on-screen size. */
  scale?: number;
  testId?: string;
  /** Regions drawn dashed, such as unsaved drafts. */
  pendingIndexes?: ReadonlySet<number>;
  /** Renders a move area and four corner handles that report presses. */
  onHandlePointerDown?: (
    index: number,
    handle: RegionHandle,
    event: ReactPointerEvent<HTMLElement>,
  ) => void;
};

const CORNERS: readonly RegionCorner[] = ["nw", "ne", "sw", "se"];

const CORNER_POSITION: Record<RegionCorner, string> = {
  nw: "top-0 left-0 cursor-nwse-resize",
  ne: "top-0 left-full cursor-nesw-resize",
  sw: "top-full left-0 cursor-nesw-resize",
  se: "top-full left-full cursor-nwse-resize",
};

function CornerHandles({
  index,
  onHandlePointerDown,
}: {
  index: number;
  onHandlePointerDown: NonNullable<Props["onHandlePointerDown"]>;
}) {
  return CORNERS.map((corner) => (
    <span
      key={corner}
      data-testid="region-handle"
      data-region-handle={corner}
      className={cn(
        "pointer-events-auto absolute flex size-4 -translate-x-1/2 -translate-y-1/2 touch-none items-center justify-center [@media(pointer:coarse)]:size-11",
        CORNER_POSITION[corner],
      )}
      onPointerDown={(event) => {
        event.stopPropagation();
        onHandlePointerDown(index, corner, event);
      }}
    >
      <span className="size-2.5 rounded-full border border-background bg-primary" />
    </span>
  ));
}

/**
 * Numbered region outlines in the host's coordinate space, plus an optional
 * dashed outline for a drag in progress.
 */
export function RegionOverlay({
  items,
  selectedIndex,
  draft,
  scale = 1,
  testId,
  pendingIndexes,
  onHandlePointerDown,
}: Props) {
  const line = 2 / scale;
  return (
    <div className="absolute top-0 left-0" aria-hidden="true">
      {items.map(({ index, rect, title }) => {
        const pending = pendingIndexes?.has(index) ?? false;
        return (
          <div
            key={index}
            data-testid={testId}
            data-region-index={index}
            data-selected={index === selectedIndex ? "true" : undefined}
            data-pending={pending ? "true" : undefined}
            title={title || undefined}
            className={cn(
              "absolute",
              pending ? "border-dashed" : "border-solid",
              index === selectedIndex ? "border-amber-500" : "border-primary",
              onHandlePointerDown ? "pointer-events-auto cursor-move touch-none" : "cursor-pointer",
            )}
            style={{
              left: rect.x,
              top: rect.y,
              width: rect.width,
              height: rect.height,
              borderWidth: line,
              forcedColorAdjust: "none",
            }}
            onPointerDown={
              onHandlePointerDown ? (event) => onHandlePointerDown(index, "move", event) : undefined
            }
            onMouseDown={onHandlePointerDown ? (event) => event.stopPropagation() : undefined}
          >
            <span
              className="absolute top-0 left-0 rounded-sm bg-primary leading-none text-primary-foreground"
              style={{ fontSize: 12 / scale, padding: `${2 / scale}px ${4 / scale}px` }}
            >
              {index + 1}
            </span>
            {onHandlePointerDown && (
              <CornerHandles index={index} onHandlePointerDown={onHandlePointerDown} />
            )}
          </div>
        );
      })}
      {draft && (
        <div
          className="pointer-events-none absolute border-dashed border-primary"
          style={{
            left: draft.x,
            top: draft.y,
            width: draft.width,
            height: draft.height,
            borderWidth: line,
          }}
        />
      )}
    </div>
  );
}
