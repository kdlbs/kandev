"use client";

import { useTranslation } from "react-i18next";
import type { CompareState } from "@/lib/question-review/compare";
import type { ReviewImage } from "@/lib/question-review/segments";
import type { Size } from "@/lib/question-review/viewport";
import { useCompareModeLabel } from "./review-compare-toolbar";
import type { ReviewDiffState } from "./use-review-diff";

type Props = {
  state: CompareState;
  a?: ReviewImage;
  b?: ReviewImage;
  sizes: Record<string, Size>;
  diff: ReviewDiffState;
};

function sameSize(a: Size, b: Size) {
  return a.width === b.width && a.height === b.height;
}

function formatPercent(changed: number, total: number, locale: string) {
  return new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 2 }).format(
    total > 0 ? changed / total : 0,
  );
}

/** Image sizes, the difference result, and a live announcement of what the stage shows. */
export function ReviewCompareStatus({ state, a, b, sizes, diff }: Props) {
  const { t, i18n } = useTranslation();
  const modeLabel = useCompareModeLabel();
  const aSize = a ? sizes[a.id] : undefined;
  const bSize = b ? sizes[b.id] : undefined;
  const shown = state.showB ? b : a;
  const announcement =
    state.mode === "toggle" && shown
      ? t(state.showB ? "task:questionReviewShowingB" : "task:questionReviewShowingA", {
          alt: shown.alt,
        })
      : modeLabel(state.mode);
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
      {aSize && bSize && !sameSize(aSize, bSize) ? (
        <span data-testid="question-review-sizes-differ" className="tabular-nums">
          {t("task:questionReviewSizesDiffer", {
            a: t("task:questionReviewImageSize", aSize),
            b: t("task:questionReviewImageSize", bSize),
          })}
        </span>
      ) : (
        aSize && <span className="tabular-nums">{t("task:questionReviewImageSize", aSize)}</span>
      )}
      {state.mode === "diff" && (
        <span data-testid="question-review-diff-status" role="status">
          {diff.status === "ready" &&
            t("task:questionReviewDiffResult", {
              count: diff.result.changed,
              percent: formatPercent(diff.result.changed, diff.result.total, i18n.language),
            })}
          {diff.status === "computing" && t("task:questionReviewDiffComputing")}
          {diff.status === "unavailable" && t("task:questionReviewDiffUnavailable")}
        </span>
      )}
      <span role="status" className="sr-only" data-testid="question-review-announcement">
        {announcement}
      </span>
    </div>
  );
}
