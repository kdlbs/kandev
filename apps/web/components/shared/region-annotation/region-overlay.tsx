"use client";

import type { PointerEvent as ReactPointerEvent } from "react";
import type { RegionCorner, RegionRect } from "@/lib/regions/geometry";
import { cn } from "@/lib/utils";
import {
  CaptureImage,
  CaptureToggle,
  DriftWarning,
  useCaptureView,
  type CaptureView,
  type RegionOverlayLabels,
} from "./region-overlay-controls";

export type { RegionOverlayLabels } from "./region-overlay-controls";

export type RegionHandle = "move" | RegionCorner;

export type RegionOverlayItem = {
  /** Position in the host's full region list; shown as `index + 1` unless `number` is set. */
  index: number;
  rect: RegionRect;
  title?: string;
  /** Label shown on the badge when the host numbers captures itself. */
  number?: number;
  /** The saved capture, which the user can show in place of the live content. */
  imageUrl?: string;
  /** Box in the host's coordinates that the outline, its controls and its hit area are cut to. */
  clip?: RegionRect;
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
  /** Regions that no longer line up with their content; drawn as a warning. */
  driftedIndexes?: ReadonlySet<number>;
  /** Enables the drift warning and saved-capture controls, with the host's copy. */
  labels?: RegionOverlayLabels;
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

/** The clip box relative to the region, so content outside it, handles included, is cut away. */
function clipPolygon(rect: RegionRect, clip: RegionRect) {
  const left = clip.x - rect.x;
  const top = clip.y - rect.y;
  const right = left + clip.width;
  const bottom = top + clip.height;
  return `polygon(${left}px ${top}px, ${right}px ${top}px, ${right}px ${bottom}px, ${left}px ${bottom}px)`;
}

const CORNER_POSITION: Record<RegionCorner, string> = {
  nw: "top-0 left-0 cursor-nwse-resize",
  ne: "top-0 left-full cursor-nesw-resize",
  sw: "top-full left-0 cursor-nesw-resize",
  se: "top-full left-full cursor-nwse-resize",
};

function CornerHandles({
  index,
  onHandlePointerDown,
  onStart,
}: {
  index: number;
  onHandlePointerDown: NonNullable<Props["onHandlePointerDown"]>;
  /** Runs before a resize is reported, like the move area's start. */
  onStart: () => void;
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
        onStart();
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
  drifted: boolean;
  labels?: RegionOverlayLabels;
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

const flag = (value: boolean) => (value ? "true" : undefined);

function outlineClass(state: {
  pending: boolean;
  drifted: boolean;
  selected: boolean;
  movable: boolean;
}) {
  return cn(
    "group absolute",
    state.pending || state.drifted ? "border-dashed" : "border-solid",
    state.selected || state.drifted ? "border-amber-500" : "border-primary",
    state.movable ? "pointer-events-auto cursor-move touch-none" : "cursor-pointer",
  );
}

/**
 * The drift warning and saved-capture toggle, centered on the top edge so they
 * never cover the number badge or a corner handle.
 */
function AlignmentControls({
  item,
  drifted,
  labels,
  view,
}: {
  item: RegionOverlayItem;
  drifted: boolean;
  labels: RegionOverlayLabels;
  view: CaptureView;
}) {
  if (!drifted && !item.imageUrl) return null;
  return (
    <span className="absolute top-0 left-1/2 flex -translate-x-1/2 -translate-y-1/2 gap-1">
      {drifted && <DriftWarning label={labels.drifted} />}
      {item.imageUrl && <CaptureToggle view={view} labels={labels} />}
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
  drifted: driftedIndex,
  labels,
  onSelect,
  onHandlePointerDown,
}: OutlineProps) {
  const { index, rect } = item;
  const view = useCaptureView();
  const drifted = driftedIndex && Boolean(labels);
  const imageUrl = labels && view.visible ? item.imageUrl : undefined;
  const startMove = onHandlePointerDown
    ? (event: ReactPointerEvent<HTMLElement>) => {
        view.hide();
        onHandlePointerDown(index, "move", event);
      }
    : undefined;
  return (
    <div
      data-testid={testId}
      data-region-index={index}
      data-selected={flag(selected)}
      data-pending={flag(pending)}
      data-busy={flag(busy)}
      data-drifted={flag(drifted)}
      title={item.title || undefined}
      className={outlineClass({ pending, drifted, selected, movable: Boolean(startMove) })}
      style={{
        left: rect.x,
        top: rect.y,
        width: rect.width,
        height: rect.height,
        borderWidth: line,
        forcedColorAdjust: "none",
        clipPath: item.clip ? clipPolygon(rect, item.clip) : undefined,
      }}
      onPointerDown={startMove}
      onMouseDown={startMove ? (event) => event.stopPropagation() : undefined}
    >
      {imageUrl && <CaptureImage url={imageUrl} />}
      <RegionBadge item={item} scale={scale} onSelect={onSelect} />
      {labels && <AlignmentControls item={item} drifted={drifted} labels={labels} view={view} />}
      {busy && (
        <span
          className="kandev-region-busy pointer-events-none absolute"
          style={{ inset: -line, padding: line * 1.5 }}
        />
      )}
      {onHandlePointerDown && (
        <CornerHandles
          index={index}
          onHandlePointerDown={onHandlePointerDown}
          onStart={view.hide}
        />
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
  driftedIndexes,
  labels,
  onSelect,
  onHandlePointerDown,
}: Props) {
  const line = 2 / scale;
  return (
    <div className="absolute top-0 left-0" aria-hidden={labels ? undefined : "true"}>
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
          drifted={driftedIndexes?.has(item.index) ?? false}
          labels={labels}
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
