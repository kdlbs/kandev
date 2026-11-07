"use client";

import type { ReviewView, Size } from "@/lib/question-review/viewport";
import { AnnotateButton, RegionOverlay } from "./review-regions";
import type { ReviewAnnotationController } from "./use-review-annotations";
import { useRegionDrawing } from "./use-region-drawing";

type PointerHandlers = Parameters<typeof useRegionDrawing>[0]["viewportHandlers"];

type Args = {
  controller: ReviewAnnotationController;
  questionId: string;
  /** The single image the stage shows, or undefined when no image is drawable. */
  imageId: string | undefined;
  image: Size;
  view: ReviewView;
  stage: Size;
  /** False in comparison modes other than Toggle. */
  allowed: boolean;
  viewportHandlers: PointerHandlers;
};

export function stageAnnotating(controller: ReviewAnnotationController, allowed: boolean) {
  return controller.annotating && allowed;
}

/** Region drawing, outlines, and the Annotate control for one stage. */
export function useStageAnnotations(args: Args) {
  const { controller, questionId } = args;
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
  return {
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
