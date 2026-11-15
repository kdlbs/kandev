"use client";

import { useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { PlanSelectionPopover } from "@/components/task/plan-selection-popover";
import type { ClarificationAnswerAnnotation } from "@/lib/types/http";
import { MAX_REGION_COMMENT, regionCommentReady } from "@/lib/question-review/regions";
import type { ReviewView } from "@/lib/question-review/viewport";
import type { ReviewAnnotationController } from "./use-review-annotations";

type Props = {
  controller: ReviewAnnotationController;
  region: ClarificationAnswerAnnotation;
  number: number;
  imageLabel: string;
  isNew: boolean;
  view: ReviewView;
  stageRef: RefObject<HTMLElement | null>;
  container: RefObject<HTMLElement | null>;
};

/** Point just below the region's bottom-left corner, in viewport pixels. */
export function regionAnchor(
  region: ClarificationAnswerAnnotation,
  view: ReviewView,
  stage: { left: number; top: number },
) {
  return {
    x: stage.left + (region.x - view.x) * view.scale,
    y: stage.top + (region.y + region.height - view.y) * view.scale,
  };
}

/**
 * The chat's selection comment popover, anchored to a region. It renders inside
 * the review dialog so the modal does not make it inert; touch screens get the
 * same bottom drawer as chat comments.
 */
export function RegionCommentPopover(props: Props) {
  const { t } = useTranslation();
  const [error, setError] = useState<string | null>(null);
  const rect = props.stageRef.current?.getBoundingClientRect();
  const position = regionAnchor(props.region, props.view, {
    left: rect?.left ?? 0,
    top: rect?.top ?? 0,
  });
  return (
    <PlanSelectionPopover
      selectedText={t("task:questionReviewRegionPreview", {
        number: props.number,
        image: props.imageLabel,
      })}
      position={position}
      editingComment={props.isNew ? undefined : props.region.comment}
      portalContainer={props.container.current}
      errorMessage={error}
      testId="question-review-region-popover"
      inputTestId="question-review-region-input"
      addButtonTestId="question-review-region-add"
      onAdd={(comment) => {
        if (!regionCommentReady(comment)) {
          setError(t("task:questionReviewRegionTooLong", { max: MAX_REGION_COMMENT }));
          return false;
        }
        props.controller.saveComment(comment);
      }}
      onDelete={
        props.isNew
          ? undefined
          : () => {
              const target = props.controller.commenting;
              if (target) props.controller.remove(target.questionId, target.index);
            }
      }
      onClose={props.controller.closeComment}
    />
  );
}
