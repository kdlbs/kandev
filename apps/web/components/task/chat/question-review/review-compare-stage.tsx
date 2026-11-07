"use client";

import { useEffect, useRef, type Dispatch } from "react";
import type { CompareAction } from "@/lib/question-review/compare";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { QuestionImageState } from "@/hooks/domains/session/use-question-images";
import type { ReviewImage as ReviewImageRef, ReviewSegment } from "@/lib/question-review/segments";
import type { Size } from "@/lib/question-review/viewport";
import { cn } from "@/lib/utils";
import { ReviewCanvas, ReviewImage } from "./review-canvas";
import { CompareLayers, WipeHandle, type LoadedReviewImage } from "./review-compare-layers";
import { ReviewCompareStatus } from "./review-compare-status";
import { ReviewCompareToolbar } from "./review-compare-toolbar";
import { ReviewZoomControls } from "./review-zoom-controls";
import { ImageState, stopStageKeys, useStageImages } from "./review-stage-shared";
import { useReviewCompare } from "./use-review-compare";
import { useReviewDiff } from "./use-review-diff";
import type { ReviewAnnotationController } from "./use-review-annotations";
import { stageAnnotating, useStageAnnotations } from "./use-stage-annotations";
import { useReviewViewport } from "./use-review-viewport";

type Props = {
  pendingId: string;
  sessionId: string;
  segment: ReviewSegment;
  annotations: ReviewAnnotationController;
};

const EMPTY_SIZE: Size = { width: 0, height: 0 };

/** Selecting a region in the list shows the image it was drawn on. */
function useShowSelectedRegion(
  annotations: ReviewAnnotationController,
  segment: ReviewSegment,
  dispatch: Dispatch<CompareAction>,
) {
  const { selected, list } = annotations;
  const imageId =
    selected?.questionId === segment.questionId
      ? list(segment.questionId)[selected.index]?.image_id
      : undefined;
  useEffect(() => {
    if (imageId && segment.images.some((image) => image.id === imageId)) {
      dispatch({ type: "show", id: imageId });
    }
  }, [dispatch, imageId, segment.images]);
}

function loaded(image: ReviewImageRef | undefined, state: QuestionImageState | undefined) {
  if (!image || state?.kind !== "loaded") return undefined;
  return { id: image.id, alt: image.alt, url: state.url, blob: state.blob };
}

export function unionSize(a: Size | undefined, b: Size | undefined): Size {
  return {
    width: Math.max(a?.width ?? 0, b?.width ?? 0),
    height: Math.max(a?.height ?? 0, b?.height ?? 0),
  };
}

function firstPending(
  pairs: [ReviewImageRef | undefined, QuestionImageState | undefined][],
): QuestionImageState | undefined | null {
  for (const [image, state] of pairs) if (image && state?.kind !== "loaded") return state;
  return null;
}

/** Compares a baseline (A) with a candidate (B) on bytes already loaded for the segment. */
// eslint-disable-next-line max-lines-per-function -- composes the stage's images, view, comparison, and regions.
export function ReviewCompareStage({ pendingId, sessionId, segment, annotations }: Props) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const { states, retry, sizes, setSize } = useStageImages(pendingId, sessionId, segment);
  const compare = useReviewCompare(segment.images);
  const { state } = compare;
  const aRef = segment.images.find((image) => image.id === state.aId);
  const bRef = segment.images.find((image) => image.id === state.bId);
  const a = loaded(aRef, states[state.aId]);
  const b = loaded(bRef, states[state.bId]);
  const stageRef = useRef<HTMLDivElement>(null);
  const areaRef = useRef<HTMLDivElement>(null);
  const toggle = state.mode === "toggle";
  const viewport = useReviewViewport(
    stageRef,
    unionSize(sizes[state.aId], sizes[state.bId]),
    areaRef,
    !stageAnnotating(annotations, toggle),
  );
  const shown = state.showB ? b : a;
  const regions = useStageAnnotations({
    controller: annotations,
    questionId: segment.questionId,
    imageId: toggle ? shown?.id : undefined,
    image: (shown && sizes[shown.id]) || EMPTY_SIZE,
    view: viewport.view,
    stage: viewport.stage,
    allowed: toggle,
    viewportHandlers: viewport.pointerHandlers,
  });
  useShowSelectedRegion(annotations, segment, compare.dispatch);
  const diff = useReviewDiff(state.mode === "diff", a?.blob, b?.blob, state.tolerance);
  const pending = firstPending([
    [aRef, states[state.aId]],
    [bRef, states[state.bId]],
  ]);
  const onStageKey = (event: { key: string; shiftKey: boolean; altKey: boolean }) =>
    regions.handleKey(event) || viewport.handleKey(event) || compare.handleKey(event);
  const overlay = pending === null ? null : <ImageState state={pending} onRetry={retry} />;
  const canvas = (pane: "a" | "b" | "both", image?: LoadedReviewImage) => (
    <ReviewCanvas
      stageRef={pane === "b" ? undefined : stageRef}
      view={viewport.view}
      onStageKey={onStageKey}
      pointerHandlers={pane === "both" ? regions.pointerHandlers : viewport.pointerHandlers}
      className={
        pane === "both" && stageAnnotating(annotations, toggle) ? "cursor-crosshair" : undefined
      }
      testId={pane === "b" ? "question-review-stage-b" : undefined}
      overlay={
        <>
          {pane === "both" && state.mode === "wipe" && a && b && (
            <WipeHandle
              value={state.wipe}
              onChange={(value) => compare.dispatch({ type: "wipe", value })}
            />
          )}
          {overlay}
        </>
      }
    >
      {pane === "both" ? (
        <>
          <CompareLayers
            state={state}
            a={a}
            b={b}
            view={viewport.view}
            stageWidth={viewport.stage.width}
            diff={diff}
            onSize={setSize}
          />
          {toggle && regions.overlay}
        </>
      ) : (
        image && (
          <ReviewImage
            key={image.url}
            src={image.url}
            alt={image.alt}
            scale={viewport.view.scale}
            onSize={(size) => setSize(image.id, size)}
          />
        )
      )}
    </ReviewCanvas>
  );
  return (
    <section
      className="flex min-h-0 flex-1 flex-col gap-2"
      aria-label={t("task:questionReviewStage")}
      onKeyDown={stopStageKeys}
    >
      <ReviewCompareToolbar compare={compare} images={segment.images} />
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <ReviewZoomControls
          scale={viewport.view.scale}
          onZoom={viewport.zoomStep}
          onFit={viewport.fit}
          onActual={viewport.actual}
        />
        {regions.button}
        <ReviewCompareStatus state={state} a={aRef} b={bRef} sizes={sizes} diff={diff} />
      </div>
      <div
        ref={areaRef}
        className={cn("flex min-h-0 flex-1 gap-2", isMobile ? "flex-col" : "flex-row")}
      >
        {state.mode === "side" ? (
          <>
            {canvas("a", a)}
            {canvas("b", b)}
          </>
        ) : (
          canvas("both")
        )}
      </div>
    </section>
  );
}
