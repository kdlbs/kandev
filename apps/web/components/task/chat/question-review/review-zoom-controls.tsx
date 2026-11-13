"use client";

import { IconMinus, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { formatZoom } from "@/lib/question-review/viewport";

/** Toolbar row: wraps on desktop, one horizontally scrolling line on phones. */
export const reviewToolbarRow =
  "flex flex-wrap items-center gap-x-3 gap-y-1.5 max-md:flex-nowrap max-md:overflow-x-auto max-md:overscroll-x-contain max-md:pb-1 [&>*]:shrink-0";

export const reviewControlSize = "h-7 cursor-pointer [@media(pointer:coarse)]:h-12 max-md:h-12";
const iconControlSize =
  "h-7 w-7 cursor-pointer [@media(pointer:coarse)]:h-12 [@media(pointer:coarse)]:w-12 max-md:h-12 max-md:w-12";

type Props = {
  scale: number;
  onZoom: (direction: 1 | -1) => void;
  onFit: () => void;
  onActual: () => void;
};

export function ReviewZoomControls({ scale, onZoom, onFit, onActual }: Props) {
  const { t } = useTranslation();
  const level = formatZoom(scale);
  return (
    <div className="flex items-center gap-1" data-testid="question-review-zoom">
      <Button
        type="button"
        variant="outline"
        size="icon"
        className={iconControlSize}
        aria-label={t("task:questionReviewZoomOut")}
        data-testid="question-review-zoom-out"
        onClick={() => onZoom(-1)}
      >
        <IconMinus className="h-4 w-4" aria-hidden="true" />
      </Button>
      <span
        className="min-w-12 text-center text-xs tabular-nums"
        data-testid="question-review-zoom-level"
      >
        {level}
      </span>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className={iconControlSize}
        aria-label={t("task:questionReviewZoomIn")}
        data-testid="question-review-zoom-in"
        onClick={() => onZoom(1)}
      >
        <IconPlus className="h-4 w-4" aria-hidden="true" />
      </Button>
      <Button
        type="button"
        variant="outline"
        className={reviewControlSize}
        data-testid="question-review-fit"
        onClick={onFit}
      >
        {t("task:questionImageFit")}
      </Button>
      <Button
        type="button"
        variant="outline"
        className={reviewControlSize}
        data-testid="question-review-actual"
        onClick={onActual}
      >
        {t("task:questionImageActualSize")}
      </Button>
      <span role="status" className="sr-only">
        {t("task:questionReviewZoomStatus", { level })}
      </span>
    </div>
  );
}
