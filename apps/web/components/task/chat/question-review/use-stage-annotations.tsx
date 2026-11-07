"use client";

import type { RefObject } from "react";
import type { ReviewSegment } from "@/lib/question-review/segments";
import type { ReviewView, Size } from "@/lib/question-review/viewport";
import { RegionCommentPopover } from "./region-comment-popover";
import { useReviewImageLabel } from "./review-image-picker";
import { AnnotateButton, RegionOverlay } from "./review-regions";
import type { ReviewAnnotationController } from "./use-review-annotations";
import { useRegionDrawing } from "./use-region-drawing";

type PointerHandlers = Parameters<typeof useRegionDrawing>[0]["viewportHandlers"];

type Args = {
  controller: ReviewAnnotationController;
  segment: ReviewSegment;
  /** The single image the stage shows, or undefined when no image is drawable. */
  imageId: string | undefined;
  image: Size;
  view: ReviewView;
  stage: Size;
  /** False in comparison modes other than Toggle. */
  allowed: boolean;
  viewportHandlers: PointerHandlers;
  stageRef: RefObject<HTMLElement | null>;
  /** The dialog element that hosts the comment popover. */
  popoverContainer: RefObject<HTMLElement | null>;
};

export function stageAnnotating(controller: ReviewAnnotationController, allowed: boolean) {
  return controller.annotating && allowed;
}

/** Region drawing, outlines, and the Annotate control for one stage. */
export function useStageAnnotations(args: Args) {
  const { controller, segment } = args;
  const questionId = segment.questionId;
  const imageLabel = useReviewImageLabel();
  const drawing = useRegionDrawing({
    active: stageAnnotating(controller, args.allowed),
    questionId,
    imageId: args.imageId,
    image: args.image,
    view: args.view,
    stage: args.stage,
    controller,
    viewportHandlers: args.viewportHandlers,
  });
  const regions = controller.list(questionId);
  const selectedIndex =
    controller.selected?.questionId === questionId ? controller.selected.index : null;
  const commenting =
    controller.commenting?.questionId === questionId ? controller.commenting : null;
  const commentRegion = commenting ? regions[commenting.index] : undefined;
  const commentImageIndex = segment.images.findIndex(
    (image) => image.id === commentRegion?.image_id,
  );
  const popover =
    commenting && commentRegion && commentRegion.image_id === args.imageId ? (
      <RegionCommentPopover
        key={`${commenting.index}:${commenting.isNew}`}
        controller={controller}
        region={commentRegion}
        number={commenting.index + 1}
        imageLabel={
          commentImageIndex >= 0
            ? imageLabel(segment.images[commentImageIndex], commentImageIndex)
            : commentRegion.image_id
        }
        isNew={commenting.isNew}
        view={args.view}
        stageRef={args.stageRef}
        container={args.popoverContainer}
      />
    ) : null;
  return {
    popover,
    pointerHandlers: drawing.pointerHandlers,
    handleKey: drawing.handleKey,
    overlay: (
      <RegionOverlay
        regions={regions}
        imageId={args.imageId}
        selectedIndex={selectedIndex}
        draft={drawing.draft}
        scale={args.view.scale}
      />
    ),
    button: (
      <AnnotateButton
        pressed={stageAnnotating(controller, args.allowed)}
        disabled={!args.allowed || !args.imageId}
        count={regions.length}
        onToggle={() => controller.setAnnotating((value) => !value)}
      />
    ),
  };
}
