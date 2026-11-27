"use client";

import { useEffect, useRef, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import type { CompareState } from "@/lib/question-review/compare";
import type { ReviewView, Size } from "@/lib/question-review/viewport";
import { wipeFromDrag } from "@/lib/question-review/wipe";
import { cn } from "@/lib/utils";
import { ReviewImage } from "./review-canvas";
import { ChangeOutline } from "./review-compare-controls";
import type { ReviewDiffResult, ReviewDiffState } from "./use-review-diff";
import {
  WipeGripHalves,
  useWipeGrip,
  wipeLineForcedColors,
  type WipeNames,
} from "./review-wipe-grip";
import type { WipeNamesReveal } from "./use-wipe-names-reveal";

export type LoadedReviewImage = { id: string; alt: string; url: string };

type Props = {
  state: CompareState;
  a?: LoadedReviewImage;
  b?: LoadedReviewImage;
  view: ReviewView;
  stageWidth: number;
  diff: ReviewDiffState;
  onSize: (id: string, size: Size) => void;
  /** Wipe line as a fraction of the stage width; the pinned touch line is 0.5. */
  wipeAt?: number;
  /** Changed-area outline: always in Difference, briefly after Zoom to changes. */
  outline?: { x: number; y: number; width: number; height: number } | null;
};

/** Image-pixel x of the wipe divider, so the clip follows zoom and pan. */
export function wipeClipLeft(view: ReviewView, stageWidth: number, wipe: number): number {
  return Math.max(0, view.x + (wipe * stageWidth) / view.scale);
}

function candidateStyle(props: Props): CSSProperties | undefined {
  const { state, view, stageWidth } = props;
  if (state.mode === "wipe") {
    const at = props.wipeAt ?? state.wipe;
    return { clipPath: `inset(0 0 0 ${wipeClipLeft(view, stageWidth, at)}px)` };
  }
  if (state.mode === "onion") return { opacity: state.opacity };
  return undefined;
}

function candidateHidden(state: CompareState): boolean {
  return state.mode === "diff" || (state.mode === "toggle" && !state.showB);
}

/** A and B share one transformed layer, so switching modes never moves the view. */
export function CompareLayers(props: Props) {
  const { state, a, b, view } = props;
  const baselineHidden = state.mode === "toggle" && state.showB;
  return (
    <div className="relative">
      {a && (
        <ReviewImage
          key={a.url}
          src={a.url}
          alt={a.alt}
          scale={view.scale}
          onSize={(size) => props.onSize(a.id, size)}
          className={baselineHidden ? "invisible" : undefined}
        />
      )}
      {b && (
        <ReviewImage
          key={b.url}
          src={b.url}
          alt={b.alt}
          scale={view.scale}
          onSize={(size) => props.onSize(b.id, size)}
          className={
            candidateHidden(state) ? "invisible absolute top-0 left-0" : "absolute top-0 left-0"
          }
          style={candidateStyle(props)}
        />
      )}
      {state.mode === "diff" && props.diff.status === "ready" && (
        <DiffMask result={props.diff.result} scale={view.scale} />
      )}
      {props.outline && <ChangeOutline bounds={props.outline} scale={view.scale} />}
    </div>
  );
}

function DiffMask({ result, scale }: { result: ReviewDiffResult; scale: number }) {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const context = ref.current?.getContext("2d");
    if (!context) return;
    const pixels = new Uint8ClampedArray(result.mask);
    context.putImageData(new ImageData(pixels, result.width, result.height), 0, 0);
  }, [result]);
  return (
    <canvas
      ref={ref}
      width={result.width}
      height={result.height}
      aria-hidden="true"
      data-testid="question-review-diff-mask"
      className="absolute top-0 left-0"
      style={{ imageRendering: scale > 1 ? "pixelated" : "auto", forcedColorAdjust: "none" }}
    />
  );
}

/**
 * Draggable and keyboard-operable divider for Wipe, in stage coordinates. The
 * line reaches the stage edges; the handle carries the names and also drags up
 * or down along the line.
 */
export function WipeHandle({
  value,
  onChange,
  names,
  reveal,
}: {
  value: number;
  onChange: (v: number) => void;
  names: WipeNames;
  reveal: WipeNamesReveal;
}) {
  const { t } = useTranslation();
  const dragStart = useRef(value);
  const handle = useWipeGrip(names, reveal, (event, dx) => {
    const stage = event.currentTarget.parentElement;
    if (!stage) return;
    const rect = stage.getBoundingClientRect();
    onChange(wipeFromDrag(dragStart.current, dx, { left: rect.left, width: rect.width }));
  });
  const percent = Math.round(value * 100);
  const onKeyDown = (event: React.KeyboardEvent) => {
    const steps: Record<string, number> = { ArrowLeft: -0.05, ArrowRight: 0.05 };
    const targets: Record<string, number> = { Home: 0, End: 1 };
    const next = event.key in targets ? targets[event.key] : value + (steps[event.key] ?? NaN);
    if (Number.isNaN(next)) return;
    event.preventDefault();
    event.stopPropagation();
    onChange(next);
  };
  return (
    <>
      <div
        className={cn("absolute inset-y-0 w-0.5 -translate-x-1/2 bg-primary", wipeLineForcedColors)}
        style={{ left: `${value * 100}%` }}
        aria-hidden="true"
      />
      <div
        role="slider"
        tabIndex={0}
        aria-label={t("task:questionReviewWipe")}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={percent}
        aria-valuetext={t("task:questionReviewPercent", { value: percent })}
        data-testid="question-review-wipe-handle"
        className="group absolute w-6 -translate-x-1/2 cursor-move touch-none text-xs outline-none [--wipe-half:12px] max-md:w-11 max-md:[--wipe-half:22px] [@media(pointer:coarse)]:w-11 [@media(pointer:coarse)]:[--wipe-half:22px]"
        {...handle.props}
        onPointerDown={(event) => {
          dragStart.current = value;
          handle.props.onPointerDown(event);
        }}
        style={{
          // At the ends the grip stays fully inside the stage, beside the line.
          left: `clamp(var(--wipe-half), ${value * 100}%, calc(100% - var(--wipe-half)))`,
          ...handle.props.style,
        }}
        onKeyDown={onKeyDown}
      >
        <WipeGripHalves grip={handle.grip} />
      </div>
    </>
  );
}
