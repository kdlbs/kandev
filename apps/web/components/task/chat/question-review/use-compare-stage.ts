import { useEffect, useRef, type Dispatch, type RefObject } from "react";
import {
  effectiveCompareState,
  type CompareAction,
  type CompareSettings,
  type CompareState,
} from "@/lib/question-review/compare";
import type { QuestionImageState } from "@/hooks/domains/session/use-question-images";
import type { ReviewImage, ReviewSegment } from "@/lib/question-review/segments";
import type { Size } from "@/lib/question-review/viewport";
import { useReviewLayout } from "./review-layout";
import { useStageImages } from "./review-stage-shared";
import type { ReviewAnnotationController } from "./use-review-annotations";
import { useReviewCompare } from "./use-review-compare";
import { useChangesZoom } from "./use-changes-zoom";
import { useHoldToCompare } from "./use-hold-to-compare";
import { stageAnnotating, useStageAnnotations } from "./use-stage-annotations";
import { useReviewViewport } from "./use-review-viewport";

export type CompareStageProps = {
  pendingId: string;
  sessionId: string;
  segment: ReviewSegment;
  annotations: ReviewAnnotationController;
  popoverContainer: RefObject<HTMLElement | null>;
  compareSettings?: CompareSettings;
  onCompareSettings?: (settings: CompareSettings) => void;
};

const EMPTY_SIZE: Size = { width: 0, height: 0 };

export function unionSize(a: Size | undefined, b: Size | undefined): Size {
  return {
    width: Math.max(a?.width ?? 0, b?.width ?? 0),
    height: Math.max(a?.height ?? 0, b?.height ?? 0),
  };
}

function loaded(image: ReviewImage | undefined, state: QuestionImageState | undefined) {
  if (!image || state?.kind !== "loaded") return undefined;
  return { id: image.id, alt: image.alt, url: state.url, blob: state.blob };
}

function firstPending(
  pairs: [ReviewImage | undefined, QuestionImageState | undefined][],
): QuestionImageState | undefined | null {
  for (const [image, state] of pairs) if (image && state?.kind !== "loaded") return state;
  return null;
}

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

type Handlers = {
  onPointerDown: (event: React.PointerEvent<HTMLElement>) => void;
  onPointerMove: (event: React.PointerEvent<HTMLElement>) => void;
  onPointerUp: (event: React.PointerEvent<HTMLElement>) => void;
  onPointerCancel: (event: React.PointerEvent<HTMLElement>) => void;
};

function combine(first: Handlers, second: Handlers): Handlers {
  return {
    onPointerDown: (event) => {
      first.onPointerDown(event);
      second.onPointerDown(event);
    },
    onPointerMove: (event) => {
      first.onPointerMove(event);
      second.onPointerMove(event);
    },
    onPointerUp: (event) => {
      first.onPointerUp(event);
      second.onPointerUp(event);
    },
    onPointerCancel: (event) => {
      first.onPointerCancel(event);
      second.onPointerCancel(event);
    },
  };
}

function stageFlags(
  state: CompareState,
  touch: boolean,
  compare: { position: { total: number } },
  annotations: ReviewAnnotationController,
) {
  const toggle = state.mode === "toggle";
  return {
    toggle,
    marking: stageAnnotating(annotations, toggle),
    pinned: touch && state.mode === "wipe",
    canStep: (toggle || state.mode === "side") && compare.position.total > 1,
  };
}

/** Everything one comparison stage needs: images, view, comparison, regions, and input. */
// eslint-disable-next-line max-lines-per-function -- one place composes the stage's interacting inputs.
export function useCompareStage(props: CompareStageProps) {
  const { segment, annotations } = props;
  const layout = useReviewLayout();
  const images = useStageImages(props.pendingId, props.sessionId, segment);
  const compare = useReviewCompare(segment.images, props.compareSettings, props.onCompareSettings);
  const { state } = compare;
  const aRef = segment.images.find((image) => image.id === state.aId);
  const bRef = segment.images.find((image) => image.id === state.bId);
  const a = loaded(aRef, images.states[state.aId]);
  const b = loaded(bRef, images.states[state.bId]);
  const { toggle, marking, pinned, canStep } = stageFlags(
    state,
    layout.touch,
    compare,
    annotations,
  );
  const hold = useHoldToCompare(!marking && Boolean(a));
  const stageRef = useRef<HTMLDivElement>(null);
  const areaRef = useRef<HTMLDivElement>(null);
  const viewport = useReviewViewport(
    stageRef,
    unionSize(images.sizes[state.aId], images.sizes[state.bId]),
    {
      areaRef,
      panEnabled: !marking,
      pinned,
      onSwipe: canStep && !marking ? compare.step : undefined,
    },
  );
  // Entering pinned Wipe recenters the line; leaving it drops the scrubbed offset.
  const fitView = viewport.fit;
  useEffect(() => {
    fitView();
  }, [pinned, fitView]);
  const effective = effectiveCompareState(state, hold.holding);
  const shown = effective.showB ? b : a;
  const regions = useStageAnnotations({
    controller: annotations,
    segment,
    stageRef,
    popoverContainer: props.popoverContainer,
    imageId: toggle ? shown?.id : undefined,
    image: (shown && images.sizes[shown.id]) || EMPTY_SIZE,
    view: viewport.view,
    stage: viewport.stage,
    allowed: toggle,
    viewportHandlers: viewport.pointerHandlers,
  });
  useShowSelectedRegion(annotations, segment, compare.dispatch);
  const changes = useChangesZoom({
    mode: state.mode,
    pairKey: `${state.aId}:${state.bId}:${state.tolerance}`,
    a: a?.blob,
    b: b?.blob,
    tolerance: state.tolerance,
    zoomToRect: viewport.zoomToRect,
  });
  const diff = changes.diff;
  const wipe = {
    value: pinned ? viewport.pinnedFraction : state.wipe,
    onChange: (value: number) =>
      pinned ? viewport.setPinnedFraction(value) : compare.dispatch({ type: "wipe", value }),
  };
  const urls = Object.fromEntries(
    Object.entries(images.states).map(([id, image]) => [
      id,
      image.kind === "loaded" ? image.url : undefined,
    ]),
  );
  return {
    ...props,
    urls,
    layout,
    images,
    compare,
    state,
    effective,
    aRef,
    bRef,
    a,
    b,
    marking,
    pinned,
    canStep,
    hold,
    stageRef,
    areaRef,
    viewport,
    regions,
    diff,
    changes,
    wipe,
    pending: firstPending([
      [aRef, images.states[state.aId]],
      [bRef, images.states[state.bId]],
    ]),
    pointerHandlers: combine(hold.pointer, regions.pointerHandlers),
    onStageKey: (event: { key: string; shiftKey: boolean; altKey: boolean; repeat: boolean }) =>
      regions.handleKey(event) ||
      hold.keyDown(event) ||
      viewport.handleKey(event) ||
      compare.handleKey(event),
    onStageKeyUp: (event: { key: string }) => hold.keyUp(event),
  };
}

export type CompareStageModel = ReturnType<typeof useCompareStage>;
