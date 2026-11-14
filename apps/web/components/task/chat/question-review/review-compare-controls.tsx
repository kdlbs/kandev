"use client";

import { IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { imageShortName, type ReviewImage } from "@/lib/question-review/segments";
import { cn } from "@/lib/utils";

const iconSize =
  "h-7 w-7 cursor-pointer [@media(pointer:coarse)]:h-12 [@media(pointer:coarse)]:w-12 max-md:h-12 max-md:w-12";

type StepProps = {
  images: readonly ReviewImage[];
  bId: string;
  position: { index: number; total: number };
  onStep: (direction: 1 | -1) => void;
  compact?: boolean;
  /** Shows "2/3" while the full label stays available to screen readers. */
  countOnly?: boolean;
};

/** Previous and Next step image B through the candidates while A stays fixed. */
export function CandidateStepper({
  images,
  bId,
  position,
  onStep,
  compact = false,
  countOnly = false,
}: StepProps) {
  const { t } = useTranslation();
  const bIndex = images.findIndex((image) => image.id === bId);
  const name = bIndex >= 0 ? imageShortName(images[bIndex], bIndex) : "";
  return (
    <div className="flex items-center gap-1" data-testid="question-review-stepper">
      <Button
        type="button"
        variant="outline"
        size="icon"
        className={iconSize}
        disabled={position.index <= 0}
        aria-label={t("task:questionReviewPreviousCandidate")}
        data-testid="question-review-step-prev"
        onClick={() => onStep(-1)}
      >
        <IconChevronLeft className="h-4 w-4" aria-hidden="true" />
      </Button>
      {countOnly && (
        <span className="text-xs text-muted-foreground tabular-nums" aria-hidden="true">
          {t("task:questionReviewCandidateCount", {
            index: position.index + 1,
            total: position.total,
          })}
        </span>
      )}
      {(!compact || countOnly) && (
        <span
          className={cn("min-w-0 truncate text-xs tabular-nums", countOnly && "sr-only")}
          data-testid="question-review-step-label"
        >
          {t("task:questionReviewCandidatePosition", {
            name,
            index: position.index + 1,
            total: position.total,
          })}
        </span>
      )}
      <Button
        type="button"
        variant="outline"
        size="icon"
        className={iconSize}
        disabled={position.index >= position.total - 1}
        aria-label={t("task:questionReviewNextCandidate")}
        data-testid="question-review-step-next"
        onClick={() => onStep(1)}
      >
        <IconChevronRight className="h-4 w-4" aria-hidden="true" />
      </Button>
    </div>
  );
}

/** Dashed outline around the changed area, in image coordinates. */
export function ChangeOutline({
  bounds,
  scale,
}: {
  bounds: { x: number; y: number; width: number; height: number };
  scale: number;
}) {
  return (
    <div
      aria-hidden="true"
      data-testid="question-review-change-outline"
      className="pointer-events-none absolute border-dashed border-amber-500"
      style={{
        left: bounds.x,
        top: bounds.y,
        width: bounds.width,
        height: bounds.height,
        borderWidth: 2 / scale,
        outline: `${1 / scale}px solid rgb(0 0 0 / 0.35)`,
        forcedColorAdjust: "none",
      }}
    />
  );
}

/**
 * Touch wipe: the line stays at the stage center and the images scrub beneath
 * it. The grip is a keyboard slider over where the line falls on the image.
 */
export function PinnedWipeLine({
  value,
  onChange,
}: {
  value: number;
  onChange: (value: number) => void;
}) {
  const { t } = useTranslation();
  const percent = Math.round(value * 100);
  return (
    <>
      <div
        className="pointer-events-none absolute inset-y-0 left-1/2 w-0.5 -translate-x-1/2 bg-primary"
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
        data-pinned="true"
        className={cn(
          "pointer-events-none absolute top-1/2 left-1/2 h-10 w-5 -translate-x-1/2 -translate-y-1/2 rounded-md border border-background bg-primary",
          "focus-visible:outline-2 focus-visible:outline-ring",
        )}
        onKeyDown={(event) => {
          const step = { ArrowLeft: -0.05, ArrowRight: 0.05 }[event.key];
          if (step === undefined) return;
          event.preventDefault();
          event.stopPropagation();
          onChange(Math.min(1, Math.max(0, value + step)));
        }}
      />
    </>
  );
}
