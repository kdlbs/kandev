"use client";

import { useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { Size } from "@/lib/question-review/viewport";
import { cn } from "@/lib/utils";
import { ReviewCanvas, ReviewImage } from "./review-canvas";
import { ReviewCompareStage } from "./review-compare-stage";
import { ReviewBottomBar, useReviewLayout } from "./review-layout";
import { MoreSheetButton, ZoomChip } from "./review-phone-controls";
import { RegionsButton } from "./review-region-panel";
import { ImageState, stopStageKeys, useStageImages } from "./review-stage-shared";
import { ReviewZoomControls, reviewToolbarRow } from "./review-zoom-controls";
import type { CompareStageProps } from "./use-compare-stage";
import { useStageAnnotations } from "./use-stage-annotations";
import { useReviewViewport } from "./use-review-viewport";

const EMPTY_SIZE: Size = { width: 0, height: 0 };

export function QuestionReviewStage(props: CompareStageProps) {
  if (props.segment.images.length > 1) return <ReviewCompareStage {...props} />;
  return <SingleImageStage {...props} />;
}

function useSingleImageStage({
  pendingId,
  sessionId,
  segment,
  annotations,
  popoverContainer,
}: CompareStageProps) {
  const images = useStageImages(pendingId, sessionId, segment);
  const selected = segment.images[0];
  const stageRef = useRef<HTMLDivElement>(null);
  const imageSize = (selected && images.sizes[selected.id]) || EMPTY_SIZE;
  const viewport = useReviewViewport(stageRef, imageSize, {
    panEnabled: !annotations.annotating,
  });
  const state = selected ? images.states[selected.id] : undefined;
  const regions = useStageAnnotations({
    controller: annotations,
    segment,
    stageRef,
    popoverContainer,
    imageId: state?.kind === "loaded" ? selected?.id : undefined,
    image: imageSize,
    view: viewport.view,
    stage: viewport.stage,
    allowed: true,
    viewportHandlers: viewport.pointerHandlers,
  });
  return { images, selected, stageRef, viewport, state, regions };
}

function SingleImageStage(props: CompareStageProps) {
  const { t } = useTranslation();
  const { phone } = useReviewLayout();
  const { images, selected, stageRef, viewport, state, regions } = useSingleImageStage(props);
  if (!selected) return <StageMessage>{t("task:questionReviewNoImages")}</StageMessage>;
  const size = images.sizes[selected.id];
  const zoom = (
    <ReviewZoomControls
      scale={viewport.view.scale}
      onZoom={viewport.zoomStep}
      onFit={viewport.fit}
      onActual={viewport.actual}
    />
  );
  return (
    <section
      className="flex min-h-0 flex-1 flex-col gap-2"
      aria-label={t("task:questionReviewStage")}
      onKeyDown={stopStageKeys}
    >
      {phone ? (
        <div className="flex items-center gap-2">
          <p className="min-w-0 flex-1 truncate text-xs text-muted-foreground">{selected.alt}</p>
          <MoreSheetButton>{() => zoom}</MoreSheetButton>
        </div>
      ) : (
        <>
          <div className={reviewToolbarRow}>
            {zoom}
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
        </>
      )}
      <ReviewCanvas
        stageRef={stageRef}
        view={viewport.view}
        onStageKey={(event) => regions.handleKey(event) || viewport.handleKey(event)}
        pointerHandlers={regions.pointerHandlers}
        className={cn(regions.marking && "cursor-crosshair ring-2 ring-primary ring-inset")}
        overlay={
          <>
            {regions.hint}
            {state?.kind !== "loaded" && <ImageState state={state} onRetry={images.retry} />}
            {phone && (
              <ZoomChip
                scale={viewport.view.scale}
                onToggle={viewport.atFit ? viewport.actual : viewport.fit}
              />
            )}
          </>
        }
      >
        {state?.kind === "loaded" && (
          <ReviewImage
            key={state.url}
            src={state.url}
            alt={selected.alt}
            scale={viewport.view.scale}
            onSize={(next) => images.setSize(selected.id, next)}
          />
        )}
        {regions.overlay}
      </ReviewCanvas>
      {phone && (
        <ReviewBottomBar>
          <div className="min-w-0 flex-1 [&>button]:w-full">{regions.button}</div>
          <RegionsButton controller={props.annotations} questionId={props.segment.questionId} />
        </ReviewBottomBar>
      )}
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
