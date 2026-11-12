"use client";

import { IconCheck, IconPencil } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { ClarificationAnswerAnnotation } from "@/lib/types/http";
import { MAX_REGIONS, type RegionRect } from "@/lib/question-review/regions";
import { cn } from "@/lib/utils";
import { reviewControlSize } from "./review-zoom-controls";

type OverlayProps = {
  regions: readonly ClarificationAnswerAnnotation[];
  imageId: string | undefined;
  selectedIndex: number | null;
  draft: RegionRect | null;
  scale: number;
};

/**
 * Region outlines in image coordinates, inside the transformed layer, so they
 * follow zoom and pan. Line width and labels are divided by the scale to keep a
 * constant on-screen size. Boxes take clicks so a region opens for editing.
 */
export function RegionOverlay({ regions, imageId, selectedIndex, draft, scale }: OverlayProps) {
  const line = 2 / scale;
  return (
    <div className="absolute top-0 left-0" aria-hidden="true">
      {regions.map((region, index) =>
        region.image_id === imageId ? (
          <div
            key={index}
            data-testid="question-review-region"
            data-region-index={index}
            data-selected={index === selectedIndex ? "true" : undefined}
            title={region.comment || undefined}
            className={cn(
              "absolute cursor-pointer border-solid",
              index === selectedIndex ? "border-amber-500" : "border-primary",
            )}
            style={{
              left: region.x,
              top: region.y,
              width: region.width,
              height: region.height,
              borderWidth: line,
              forcedColorAdjust: "none",
            }}
          >
            <span
              className="absolute top-0 left-0 rounded-sm bg-primary leading-none text-primary-foreground"
              style={{ fontSize: 12 / scale, padding: `${2 / scale}px ${4 / scale}px` }}
            >
              {index + 1}
            </span>
          </div>
        ) : null,
      )}
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

/** Mark area toggle: a pen icon, and solid primary with a check while marking. */
export function AnnotateButton({
  pressed,
  disabled,
  count,
  onToggle,
}: {
  pressed: boolean;
  disabled: boolean;
  count: number;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  const Icon = pressed ? IconCheck : IconPencil;
  return (
    <Button
      type="button"
      variant={pressed ? "default" : "outline"}
      className={reviewControlSize}
      aria-pressed={pressed}
      disabled={disabled}
      title={
        count >= MAX_REGIONS ? t("task:questionReviewRegionLimit", { max: MAX_REGIONS }) : undefined
      }
      data-testid="question-review-annotate"
      onClick={onToggle}
    >
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
      {t(pressed ? "task:questionReviewMarking" : "task:questionReviewMarkArea")}
    </Button>
  );
}

/** Shown on the image while marking, so the mode is obvious and easy to leave. */
export function AnnotateHint({ touch, onDone }: { touch: boolean; onDone: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      className="pointer-events-auto absolute top-2 left-2 z-10 flex max-w-[calc(100%-12rem)] items-center gap-2 rounded-full border border-primary/40 bg-background/95 py-1 pr-1 pl-3 text-xs shadow-sm max-md:max-w-[calc(100%-1rem)]"
      data-testid="question-review-annotate-hint"
      onPointerDown={(event) => event.stopPropagation()}
    >
      <span className="min-w-0 truncate">
        {t(touch ? "task:questionReviewMarkHintTouch" : "task:questionReviewMarkHint")}
      </span>
      <Button
        type="button"
        size="sm"
        className="h-7 cursor-pointer rounded-full max-md:h-11 [@media(pointer:coarse)]:h-11"
        data-testid="question-review-annotate-done"
        onClick={onDone}
      >
        {t("task:questionReviewMarkDone")}
      </Button>
    </div>
  );
}
