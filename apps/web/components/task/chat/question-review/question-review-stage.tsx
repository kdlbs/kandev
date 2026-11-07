"use client";

import { useRef, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import type { ReviewSegment } from "@/lib/question-review/segments";
import type { Size } from "@/lib/question-review/viewport";
import { ReviewCanvas, ReviewImage } from "./review-canvas";
import { ReviewCompareStage } from "./review-compare-stage";
import { ImageState, stopStageKeys, useStageImages } from "./review-stage-shared";
import { ReviewZoomControls, reviewToolbarRow } from "./review-zoom-controls";
import type { ReviewAnnotationController } from "./use-review-annotations";
import { stageAnnotating, useStageAnnotations } from "./use-stage-annotations";
import { useReviewViewport } from "./use-review-viewport";

type Props = {
  pendingId: string;
  sessionId: string;
  segment: ReviewSegment;
  annotations: ReviewAnnotationController;
  popoverContainer: RefObject<HTMLElement | null>;
};

const EMPTY_SIZE: Size = { width: 0, height: 0 };

export function QuestionReviewStage(props: Props) {
  if (props.segment.images.length > 1) return <ReviewCompareStage {...props} />;
  return <SingleImageStage {...props} />;
}

function SingleImageStage({ pendingId, sessionId, segment, annotations, popoverContainer }: Props) {
  const { t } = useTranslation();
  const { states, retry, sizes, setSize } = useStageImages(pendingId, sessionId, segment);
  const selected = segment.images[0];
  const stageRef = useRef<HTMLDivElement>(null);
  const imageSize = (selected && sizes[selected.id]) || EMPTY_SIZE;
  const viewport = useReviewViewport(
    stageRef,
    imageSize,
    undefined,
    !stageAnnotating(annotations, true),
  );
  const loadedState = selected ? states[selected.id] : undefined;
  const regions = useStageAnnotations({
    controller: annotations,
    segment,
    stageRef,
    popoverContainer,
    imageId: loadedState?.kind === "loaded" ? selected?.id : undefined,
    image: imageSize,
    view: viewport.view,
    stage: viewport.stage,
    allowed: true,
    viewportHandlers: viewport.pointerHandlers,
  });
  if (!selected) return <StageMessage>{t("task:questionReviewNoImages")}</StageMessage>;
  const state = states[selected.id];
  const size = sizes[selected.id];
  return (
    <section
      className="flex min-h-0 flex-1 flex-col gap-2"
      aria-label={t("task:questionReviewStage")}
      onKeyDown={stopStageKeys}
    >
      <div className={reviewToolbarRow}>
        <ReviewZoomControls
          scale={viewport.view.scale}
          onZoom={viewport.zoomStep}
          onFit={viewport.fit}
          onActual={viewport.actual}
        />
        {regions.button}
        {size && (
          <span className="text-xs text-muted-foreground tabular-nums">
            {t("task:questionReviewImageSize", size)}
          </span>
        )}
      </div>
      <p className="min-w-0 truncate text-xs text-muted-foreground" title={selected.alt}>
        {selected.alt}
      </p>
      <ReviewCanvas
        stageRef={stageRef}
        view={viewport.view}
        onStageKey={(event) => regions.handleKey(event) || viewport.handleKey(event)}
        pointerHandlers={regions.pointerHandlers}
        className={stageAnnotating(annotations, true) ? "cursor-crosshair" : undefined}
        overlay={state?.kind === "loaded" ? null : <ImageState state={state} onRetry={retry} />}
      >
        {state?.kind === "loaded" && (
          <ReviewImage
            key={state.url}
            src={state.url}
            alt={selected.alt}
            scale={viewport.view.scale}
            onSize={(next) => setSize(selected.id, next)}
          />
        )}
        {regions.overlay}
      </ReviewCanvas>
      {regions.popover}
    </section>
  );
}

function StageMessage({ children }: { children: ReactNode }) {
  return (
    <div
      className="flex min-h-0 flex-1 items-center justify-center rounded-md bg-muted/40 p-4 text-sm text-muted-foreground"
      data-testid="question-review-stage-empty"
    >
      {children}
    </div>
  );
}
