"use client";

import { useEffect, useRef, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import type { CompareState } from "@/lib/question-review/compare";
import type { ReviewView, Size } from "@/lib/question-review/viewport";
import { wipeFromPointer } from "@/lib/question-review/wipe";
import { ReviewImage } from "./review-canvas";
import { ChangeOutline } from "./review-compare-controls";
import type { ReviewDiffResult, ReviewDiffState } from "./use-review-diff";

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
 * line reaches the stage edges; the handle stays fully inside the stage.
 */
export function WipeHandle({ value, onChange }: { value: number; onChange: (v: number) => void }) {
  const { t } = useTranslation();
  const percent = Math.round(value * 100);
  const moveTo = (element: HTMLElement, clientX: number) => {
    const stage = element.parentElement;
    if (!stage) return;
    const rect = stage.getBoundingClientRect();
    onChange(wipeFromPointer(clientX, { left: rect.left, width: rect.width }));
  };
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
        className="absolute inset-y-0 w-0.5 -translate-x-1/2 bg-primary"
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
        className="absolute top-1/2 h-12 w-6 -translate-x-1/2 -translate-y-1/2 cursor-ew-resize touch-none rounded-md border border-background bg-primary [--wipe-half:12px] focus-visible:outline-2 focus-visible:outline-ring max-md:w-11 max-md:[--wipe-half:22px] [@media(pointer:coarse)]:w-11 [@media(pointer:coarse)]:[--wipe-half:22px]"
        style={{ left: `clamp(var(--wipe-half), ${value * 100}%, calc(100% - var(--wipe-half)))` }}
        onKeyDown={onKeyDown}
        onPointerDown={(event) => {
          event.stopPropagation();
          event.currentTarget.setPointerCapture?.(event.pointerId);
        }}
        onPointerMove={(event) => {
          if (!event.currentTarget.hasPointerCapture?.(event.pointerId)) return;
          event.stopPropagation();
          moveTo(event.currentTarget, event.clientX);
        }}
      />
    </>
  );
}
