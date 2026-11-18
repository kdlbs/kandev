"use client";

import type { PointerEvent as ReactPointerEvent } from "react";
import type { RegionCorner, RegionRect } from "@/lib/regions/geometry";
import { cn } from "@/lib/utils";

export type RegionHandle = "move" | RegionCorner;

export type RegionOverlayItem = {
  /** Position in the host's full region list; shown as `index + 1` unless `number` is set. */
  index: number;
  rect: RegionRect;
  title?: string;
  /** Label shown on the badge when the host numbers captures itself. */
  number?: number;
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
  /** Regions still being captured; drawn with an animated border. */
  busyIndexes?: ReadonlySet<number>;
  /** Makes each number badge clickable, independent of the move and resize handles. */
  onSelect?: (index: number) => void;
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

type OutlineProps = {
  item: RegionOverlayItem;
  line: number;
  scale: number;
  testId?: string;
  selected: boolean;
  pending: boolean;
  busy: boolean;
  onSelect?: Props["onSelect"];
  onHandlePointerDown?: Props["onHandlePointerDown"];
};

function RegionBadge({ item, scale, onSelect }: Pick<OutlineProps, "item" | "scale" | "onSelect">) {
  return (
    <span
      data-region-badge=""
      className={cn(
        "absolute top-0 left-0 rounded-sm bg-primary leading-none text-primary-foreground",
        onSelect && "pointer-events-auto cursor-pointer",
      )}
      style={{ fontSize: 12 / scale, padding: `${2 / scale}px ${4 / scale}px` }}
      onPointerDown={onSelect ? (event) => event.stopPropagation() : undefined}
      onClick={
        onSelect
          ? (event) => {
              event.stopPropagation();
              onSelect(item.index);
            }
          : undefined
      }
    >
      {item.number ?? item.index + 1}
    </span>
  );
}

function RegionOutline({
  item,
  line,
  scale,
  testId,
  selected,
  pending,
  busy,
  onSelect,
  onHandlePointerDown,
}: OutlineProps) {
  const { index, rect, title } = item;
  return (
    <div
      data-testid={testId}
      data-region-index={index}
      data-selected={selected ? "true" : undefined}
      data-pending={pending ? "true" : undefined}
      data-busy={busy ? "true" : undefined}
      title={title || undefined}
      className={cn(
        "absolute",
        pending ? "border-dashed" : "border-solid",
        selected ? "border-amber-500" : "border-primary",
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
      <RegionBadge item={item} scale={scale} onSelect={onSelect} />
      {busy && (
        <span
          className="kandev-region-busy pointer-events-none absolute"
          style={{ inset: -line, padding: line * 1.5 }}
        />
      )}
      {onHandlePointerDown && (
        <CornerHandles index={index} onHandlePointerDown={onHandlePointerDown} />
      )}
    </div>
  );
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
  busyIndexes,
  onSelect,
  onHandlePointerDown,
}: Props) {
  const line = 2 / scale;
  return (
    <div className="absolute top-0 left-0" aria-hidden="true">
      {items.map((item) => (
        <RegionOutline
          key={item.index}
          item={item}
          line={line}
          scale={scale}
          testId={testId}
          selected={item.index === selectedIndex}
          pending={pendingIndexes?.has(item.index) ?? false}
          busy={busyIndexes?.has(item.index) ?? false}
          onSelect={onSelect}
          onHandlePointerDown={onHandlePointerDown}
        />
      ))}
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
