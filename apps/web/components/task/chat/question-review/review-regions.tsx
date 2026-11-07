"use client";

import { IconSquareDashed } from "@tabler/icons-react";
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
 * constant on-screen size.
 */
export function RegionOverlay({ regions, imageId, selectedIndex, draft, scale }: OverlayProps) {
  const line = 2 / scale;
  return (
    <div className="pointer-events-none absolute top-0 left-0" aria-hidden="true">
      {regions.map((region, index) =>
        region.image_id === imageId ? (
          <div
            key={index}
            data-testid="question-review-region"
            data-selected={index === selectedIndex ? "true" : undefined}
            className={cn(
              "absolute border-solid",
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
          className="absolute border-dashed border-primary"
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
  return (
    <Button
      type="button"
      variant={pressed ? "secondary" : "outline"}
      className={reviewControlSize}
      aria-pressed={pressed}
      disabled={disabled}
      title={
        count >= MAX_REGIONS ? t("task:questionReviewRegionLimit", { max: MAX_REGIONS }) : undefined
      }
      data-testid="question-review-annotate"
      onClick={onToggle}
    >
      <IconSquareDashed className="h-3.5 w-3.5" aria-hidden="true" />
      {t("task:questionReviewAnnotate")}
    </Button>
  );
}
