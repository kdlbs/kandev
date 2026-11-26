"use client";

import { useEffect, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { ReviewImage as ReviewImageRef } from "@/lib/question-review/segments";
import { useReviewImageLabel } from "./review-image-picker";
import type { Size } from "@/lib/question-review/viewport";
import { cn } from "@/lib/utils";
import { ReviewCanvas, ReviewImage } from "./review-canvas";
import { ReviewCompareStage } from "./review-compare-stage";
import { ReviewBottomBar, useReviewLayout } from "./review-layout";
import { RegionsButton } from "./review-region-panel";
import { ImageState, StageBottomRow, stopStageKeys, useStageImages } from "./review-stage-shared";
import { CompareImagesButton, ItemNav, ItemNavRow } from "./review-item-nav";
import { StageOverlayControl } from "./review-desktop-controls";
import { StageZoom } from "./review-stage-zoom";
import type { CompareStageProps, ItemStageProps } from "./use-compare-stage";
import { useRegionReveal } from "./use-region-reveal";
import { useReviewItems } from "./use-review-items";
import { useStageAnnotations } from "./use-stage-annotations";
import { useRememberedView } from "./use-remembered-view";
import { useReviewViewport } from "./use-review-viewport";

const EMPTY_SIZE: Size = { width: 0, height: 0 };

/**
 * One segment's stage. It loads the segment's images once and shows its items:
 * a pair with comparison tools, a single or gallery image alone, or a
 * comparison the operator started from the gallery.
 */
export function QuestionReviewStage(props: CompareStageProps) {
  const images = useStageImages(props.pendingId, props.sessionId, props.segment);
  const items = useReviewItems(props.segment);
  const reveal = useRegionReveal(
    props.annotations,
    props.segment,
    items.showImage,
    props.imageRequest,
  );
  const comparing = Boolean(items.compare) || items.item?.kind === "pair";
  const { imageId: revealing, consume } = reveal;
  // A single or gallery item already shows the revealed image.
  useEffect(() => {
    if (revealing && !comparing) consume();
  }, [comparing, consume, revealing]);
  const focus = useKeepStageFocus(`${items.index}:${items.compare ? "compare" : "item"}`);
  return (
    <div
      ref={focus.containerRef}
      className="flex min-h-0 flex-1 flex-col"
      onFocusCapture={focus.onFocusCapture}
      onBlurCapture={focus.onBlurCapture}
    >
      <ItemStage {...props} images={images} items={items} reveal={reveal} />
    </div>
  );
}

/**
 * Each item mounts its own stage, so moving between items removes the focused
 * canvas. Focus that was in the stage moves to the new item's canvas.
 */
function useKeepStageFocus(itemKey: string) {
  const containerRef = useRef<HTMLDivElement>(null);
  const inside = useRef(false);
  useEffect(() => {
    const container = containerRef.current;
    if (!inside.current || !container || container.contains(document.activeElement)) return;
    container.querySelector<HTMLElement>('[data-testid="question-review-stage"]')?.focus();
  }, [itemKey]);
  return {
    containerRef,
    onFocusCapture: () => {
      inside.current = true;
    },
    onBlurCapture: (event: React.FocusEvent) => {
      const next = event.relatedTarget as Node | null;
      if (next && !containerRef.current?.contains(next)) inside.current = false;
    },
  };
}

function ItemStage(shared: ItemStageProps) {
  const { t } = useTranslation();
  const { items, segment } = shared;
  const { item } = items;
  if (items.compare) {
    return (
      <ReviewCompareStage
        key="compare"
        {...shared}
        compareImages={segment.images}
        pair={false}
        initial={items.compare}
      />
    );
  }
  if (!item) return <StageMessage>{t("task:questionReviewNoImages")}</StageMessage>;
  if (item.kind === "pair") {
    return (
      <ReviewCompareStage
        key={`pair:${items.index}`}
        {...shared}
        compareImages={item.images}
        pair
      />
    );
  }
  return <SingleImageStage key={`image:${items.index}`} {...shared} image={item.image} />;
}

type SingleStageProps = ItemStageProps & { image: ReviewImageRef };

function useSingleImageStage({
  images,
  image: selected,
  segment,
  annotations,
  popoverContainer,
  carry,
}: SingleStageProps) {
  const stageRef = useRef<HTMLDivElement>(null);
  const imageSize = (selected && images.sizes[selected.id]) || EMPTY_SIZE;
  const viewport = useReviewViewport(stageRef, imageSize, {
    panEnabled: !annotations.annotating,
  });
  useRememberedView(viewport, carry, `${segment.questionId}:image:${selected.id}`, imageSize);
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

function SingleImageStage(props: SingleStageProps) {
  const { t } = useTranslation();
  const { phone } = useReviewLayout();
  const { images, selected, stageRef, viewport, state, regions } = useSingleImageStage(props);
  const size = images.sizes[selected.id];
  const caption = useImageCaption(selected, props.segment.images);
  return (
    <section
      className="flex min-h-0 flex-1 flex-col gap-2"
      aria-label={t("task:questionReviewStage")}
      onKeyDown={stopStageKeys}
    >
      {phone ? (
        <div className="flex items-center gap-2">
          <p className="min-w-0 flex-1 truncate text-xs text-muted-foreground">{caption}</p>
          <CompareImagesButton items={props.items} short />
        </div>
      ) : (
        <div className="flex min-w-0 items-center gap-3">
          <ItemNav items={props.items} />
          <CompareImagesButton items={props.items} />
          <p className="min-w-0 truncate text-xs text-muted-foreground" title={caption}>
            {caption}
            {size && (
              <span className="ml-2 tabular-nums">{t("task:questionReviewImageSize", size)}</span>
            )}
          </p>
        </div>
      )}
      <ReviewCanvas
        stageRef={stageRef}
        view={viewport.view}
        onStageKey={(event) =>
          props.items.handleKey(event) || regions.handleKey(event) || viewport.handleKey(event)
        }
        pointerHandlers={regions.pointerHandlers}
        className={cn(regions.marking && "cursor-crosshair ring-2 ring-primary ring-inset")}
        onDoubleClick={viewport.atFit ? viewport.actual : viewport.fit}
        overlay={
          <>
            {regions.hint}
            {state?.kind !== "loaded" && <ImageState state={state} onRetry={images.retry} />}
            <StageBottomRow
              end={
                <StageZoom viewport={viewport} phone={phone} container={props.popoverContainer} />
              }
            />
            {!phone && <StageOverlayControl>{regions.button}</StageOverlayControl>}
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
      {phone && <ItemNavRow items={props.items} />}
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

/**
 * The image's description, plus its number and owning choice for a choice
 * image, so two choices with the same description stay distinct.
 */
function useImageCaption(image: ReviewImageRef, all: readonly ReviewImageRef[]) {
  const imageLabel = useReviewImageLabel();
  if (image.choiceLabel === undefined) return image.alt;
  return imageLabel(
    image,
    all.findIndex((candidate) => candidate.id === image.id),
  );
}
