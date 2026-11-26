"use client";

import { Button } from "@kandev/ui/button";
import { Slider } from "@kandev/ui/slider";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import type { ReactNode, RefObject } from "react";
import { useTranslation } from "react-i18next";
import { COMPARE_MODES, MAX_TOLERANCE, type CompareMode } from "@/lib/question-review/compare";
import type { ReviewImage } from "@/lib/question-review/segments";
import { ImagePairControl } from "./review-desktop-controls";
import { reviewControlSize, reviewToolbarRow } from "./review-zoom-controls";
import type { ReviewCompare } from "./use-review-compare";
import type { WipeNamesReveal } from "./use-wipe-names-reveal";

const MODE_LABEL_KEYS: Record<CompareMode, string> = {
  toggle: "task:questionReviewModeToggle",
  wipe: "task:questionReviewModeWipe",
  side: "task:questionReviewModeSide",
  onion: "task:questionReviewModeOnion",
  diff: "task:questionReviewModeDiff",
};

export function useCompareModeLabel() {
  const { t } = useTranslation();
  return (mode: CompareMode) => t(MODE_LABEL_KEYS[mode]);
}

const toggleItemSize = "h-7 cursor-pointer px-2 text-xs [@media(pointer:coarse)]:h-12 max-md:h-12";

type WipeControl = { value: number; onChange: (value: number) => void; reveal: WipeNamesReveal };

type Props = {
  compare: ReviewCompare;
  images: readonly ReviewImage[];
  wipe: WipeControl;
  urls: Record<string, string | undefined>;
  /** Item navigation or Back to gallery, before the modes. */
  leading?: ReactNode;
  /** A pair keeps its declared order: no pickers, Swap, or candidate stepping. */
  pair?: boolean;
  /** Where the pickers' lists render: inside the review dialog. */
  container?: RefObject<HTMLElement | null>;
};

export function ReviewCompareToolbar(props: Props) {
  const { compare, images, wipe, urls, leading, pair, container } = props;
  const { t } = useTranslation();
  const modeLabel = useCompareModeLabel();
  const { state, dispatch } = compare;
  return (
    <div className={reviewToolbarRow} data-testid="question-review-compare">
      {leading}
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        aria-label={t("task:questionReviewCompareModes")}
        value={state.mode}
        onValueChange={(mode) => mode && dispatch({ type: "mode", mode: mode as CompareMode })}
        data-testid="question-review-modes"
        className="flex-wrap max-md:flex-nowrap"
      >
        {COMPARE_MODES.map((mode) => (
          <ToggleGroupItem key={mode} value={mode} className={toggleItemSize}>
            {modeLabel(mode)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      {!pair && (
        <ImagePairControl
          images={images}
          aId={state.aId}
          bId={state.bId}
          onPick={(slot, id) => dispatch({ type: "pick", slot, id })}
          onSwap={() => dispatch({ type: "swap" })}
          urls={urls}
          container={container}
          stepper={
            state.mode === "toggle" || state.mode === "side"
              ? { position: compare.position, onStep: compare.step }
              : undefined
          }
        />
      )}
      <ModeControls compare={compare} wipe={wipe} />
    </div>
  );
}

export function ModeControls({ compare, wipe }: { compare: ReviewCompare; wipe: WipeControl }) {
  const { t } = useTranslation();
  const { state, dispatch } = compare;
  if (state.mode === "toggle") {
    return (
      <Button
        type="button"
        className={reviewControlSize}
        aria-pressed={state.showB}
        data-testid="question-review-flip"
        onClick={() => dispatch({ type: "flip" })}
      >
        {t("task:questionReviewFlip")}
      </Button>
    );
  }
  if (state.mode === "wipe") {
    return (
      <LabelledSlider
        label={t("task:questionReviewWipe")}
        value={Math.round(wipe.value * 100)}
        max={100}
        valueText={t("task:questionReviewPercent", { value: Math.round(wipe.value * 100) })}
        onChange={(value) => wipe.onChange(value / 100)}
        onHover={(on) => wipe.reveal.hold("slider", on)}
        testId="question-review-wipe-slider"
      />
    );
  }
  if (state.mode === "onion") {
    return (
      <LabelledSlider
        label={t("task:questionReviewOpacity")}
        value={Math.round(state.opacity * 100)}
        max={100}
        valueText={t("task:questionReviewPercent", { value: Math.round(state.opacity * 100) })}
        onChange={(value) => dispatch({ type: "opacity", value: value / 100 })}
        testId="question-review-opacity"
      />
    );
  }
  if (state.mode === "diff") {
    return (
      <LabelledSlider
        label={t("task:questionReviewTolerance")}
        value={state.tolerance}
        max={MAX_TOLERANCE}
        valueText={String(state.tolerance)}
        onChange={(value) => dispatch({ type: "tolerance", value })}
        testId="question-review-tolerance"
      />
    );
  }
  return null;
}

function LabelledSlider(props: {
  label: string;
  value: number;
  max: number;
  valueText: string;
  onChange: (value: number) => void;
  /** Pointer over the slider, including a touch held on it. */
  onHover?: (on: boolean) => void;
  testId: string;
}) {
  return (
    <div
      className="flex min-w-48 items-center gap-2 text-xs"
      data-testid={props.testId}
      onPointerEnter={() => props.onHover?.(true)}
      onPointerLeave={() => props.onHover?.(false)}
      onPointerCancel={() => props.onHover?.(false)}
    >
      <span className="text-muted-foreground" aria-hidden="true">
        {props.label}
      </span>
      <Slider
        className="w-32 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        min={0}
        max={props.max}
        step={1}
        value={[props.value]}
        aria-label={props.label}
        aria-valuetext={props.valueText}
        onValueChange={([value]) => props.onChange(value)}
      />
      <span className="w-10 tabular-nums">{props.valueText}</span>
    </div>
  );
}
