"use client";

import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { useTranslation } from "react-i18next";
import type { ReviewSegment } from "@/lib/question-review/segments";
import { MAX_REGIONS, MAX_REGION_COMMENT, regionCommentReady } from "@/lib/question-review/regions";
import { cn } from "@/lib/utils";
import { useReviewImageLabel } from "./review-image-picker";
import { reviewControlSize } from "./review-zoom-controls";
import type { ReviewAnnotationController } from "./use-review-annotations";

type Props = { segment: ReviewSegment; controller: ReviewAnnotationController };

/** Text list of the active question's regions, so every region can be reviewed and edited. */
export function ReviewRegionList({ segment, controller }: Props) {
  const { t } = useTranslation();
  const imageLabel = useReviewImageLabel();
  const regions = controller.list(segment.questionId);
  if (segment.images.length === 0) return null;
  const selectedIndex =
    controller.selected?.questionId === segment.questionId ? controller.selected.index : null;
  const missingComment = regions.some((region) => !regionCommentReady(region.comment));
  const headingId = `question-review-regions-${segment.index}`;
  return (
    <section
      aria-labelledby={headingId}
      className="mx-4 mb-4 space-y-2 border-t border-border pt-3"
      data-testid="question-review-regions"
    >
      <h3 id={headingId} className="text-xs font-medium">
        {t("task:questionReviewRegions", { count: regions.length, max: MAX_REGIONS })}
      </h3>
      {regions.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("task:questionReviewRegionsHint")}</p>
      )}
      <ol className="space-y-2">
        {regions.map((region, index) => {
          const imageIndex = segment.images.findIndex((image) => image.id === region.image_id);
          const image = segment.images[imageIndex];
          const ready = regionCommentReady(region.comment);
          return (
            <li
              key={index}
              data-testid="question-review-region-item"
              data-selected={index === selectedIndex ? "true" : undefined}
              className={cn(
                "space-y-1.5 rounded-md border p-2",
                index === selectedIndex ? "border-amber-500" : "border-border",
              )}
            >
              <div className="flex flex-wrap items-center gap-2 text-xs">
                <span className="font-medium tabular-nums">{index + 1}</span>
                <span className="min-w-0 flex-1 truncate text-muted-foreground">
                  {image ? imageLabel(image, imageIndex) : region.image_id}
                </span>
                <Button
                  type="button"
                  variant="outline"
                  className={reviewControlSize}
                  aria-pressed={index === selectedIndex}
                  data-testid="question-review-region-select"
                  onClick={() => controller.setSelected({ questionId: segment.questionId, index })}
                >
                  {t("task:questionReviewRegionSelect", { number: index + 1 })}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  className={reviewControlSize}
                  data-testid="question-review-region-delete"
                  onClick={() => controller.remove(segment.questionId, index)}
                >
                  {t("task:questionReviewRegionDelete", { number: index + 1 })}
                </Button>
              </div>
              <Textarea
                value={region.comment}
                rows={2}
                aria-label={t("task:questionReviewRegionComment", { number: index + 1 })}
                aria-invalid={!ready}
                data-testid="question-review-region-comment"
                className="min-h-12 text-xs"
                onChange={(event) =>
                  controller.update(segment.questionId, index, { comment: event.target.value })
                }
              />
              <p className="text-right text-[11px] text-muted-foreground tabular-nums">
                {t("task:questionReviewRegionCount", {
                  length: Array.from(region.comment.trim()).length,
                  max: MAX_REGION_COMMENT,
                })}
              </p>
            </li>
          );
        })}
      </ol>
      {missingComment && (
        <p
          role="alert"
          className="text-xs text-destructive"
          data-testid="question-review-region-error"
        >
          {t("task:questionReviewRegionNeedsComment")}
        </p>
      )}
    </section>
  );
}
