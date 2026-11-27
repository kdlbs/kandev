import { useEffect, useRef, type Dispatch, type RefObject } from "react";
import {
  effectiveCompareState,
  type CompareAction,
  type CompareState,
} from "@/lib/question-review/compare";
import type { QuestionImageState } from "@/hooks/domains/session/use-question-images";
import {
  imageShortName,
  type ReviewImage,
  type ReviewSegment,
} from "@/lib/question-review/segments";
import { measuredPairSize, type Size } from "@/lib/question-review/viewport";
import { useReviewLayout } from "./review-layout";
import type { StageImages } from "./review-stage-shared";
import type { ReviewAnnotationController } from "./use-review-annotations";
import { useReviewCompare } from "./use-review-compare";
import { useRememberedView } from "./use-remembered-view";
import type { ImageRequest } from "./review-image-request";
import type { ReviewCarry } from "@/lib/question-review/review-carry";
import type { RegionReveal } from "./use-region-reveal";
import type { OperatorCompare, ReviewItems } from "./use-review-items";
import { useChangesZoom } from "./use-changes-zoom";
import { useHoldToCompare } from "./use-hold-to-compare";
import { stageAnnotating, useStageAnnotations } from "./use-stage-annotations";
import { useReviewViewport } from "./use-review-viewport";
import { usePulseOnSplitMove, useWipeNamesReveal } from "./use-wipe-names-reveal";

export type CompareStageProps = {
  pendingId: string;
  sessionId: string;
  segment: ReviewSegment;
  annotations: ReviewAnnotationController;
  popoverContainer: RefObject<HTMLElement | null>;
  carry?: ReviewCarry;
  /** The answer form asked to show this image (Show image on a choice). */
  imageRequest?: ImageRequest | null;
};

/** One item's stage: the segment's loaded images, its item navigation, and a pending region reveal. */
export type ItemStageProps = CompareStageProps & {
  images: StageImages;
  items: ReviewItems;
  reveal: RegionReveal;
};

/** A comparison of a pair, or of any two segment images the operator chose. */
export type CompareItemProps = ItemStageProps & {
  compareImages: readonly ReviewImage[];
  /** A pair fixes its two images: no pickers and no candidates. */
  pair: boolean;
  initial?: OperatorCompare;
};

const EMPTY_SIZE: Size = { width: 0, height: 0 };

export function unionSize(a: Size | undefined, b: Size | undefined): Size {
  return {
    width: Math.max(a?.width ?? 0, b?.width ?? 0),
    height: Math.max(a?.height ?? 0, b?.height ?? 0),
  };
}

/** An image's short name by its segment position, so numbers match the form's references. */
export function segmentImageName(segment: ReviewSegment, id: string): string {
  const index = segment.images.findIndex((image) => image.id === id);
  return index >= 0 ? imageShortName(segment.images[index], index) : "";
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

/** Shows a revealed region's image once, then marks the reveal handled. */
function useApplyReveal(
  reveal: RegionReveal,
  compareImages: readonly ReviewImage[],
  dispatch: Dispatch<CompareAction>,
) {
  const { imageId, consume } = reveal;
  useEffect(() => {
    if (!imageId) return;
    if (compareImages.some((image) => image.id === imageId))
      dispatch({ type: "show", id: imageId });
    consume();
  }, [compareImages, consume, dispatch, imageId]);
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
export function useCompareStage(props: CompareItemProps) {
  const { segment, annotations, images, compareImages, items } = props;
  const layout = useReviewLayout();
  const compare = useReviewCompare(compareImages, props.carry, {
    initial: props.initial,
    fixed: props.pair,
  });
  const { state } = compare;
  const aRef = compareImages.find((image) => image.id === state.aId);
  const bRef = compareImages.find((image) => image.id === state.bId);
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
  const viewport = useCompareViewport({
    stageRef,
    areaRef,
    sizes: [images.sizes[state.aId], images.sizes[state.bId]],
    marking,
    pinned,
    compare,
    carry: props.carry,
    viewKey: stageViewKey(segment.questionId, props),
  });
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
  useApplyReveal(props.reveal, compareImages, compare.dispatch);
  const changes = useChangesZoom({
    mode: state.mode,
    pairKey: `${state.aId}:${state.bId}:${state.tolerance}`,
    a: a?.blob,
    b: b?.blob,
    tolerance: state.tolerance,
    zoomToRect: viewport.zoomToRect,
  });
  const diff = changes.diff;
  const wipe = useStageWipe(viewport, compare, pinned, effective.mode === "wipe");
  const urls = Object.fromEntries(
    Object.entries(images.states).map(([id, image]) => [
      id,
      image.kind === "loaded" ? image.url : undefined,
    ]),
  );
  return {
    ...props,
    names: { a: segmentImageName(segment, state.aId), b: segmentImageName(segment, state.bId) },
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
      items.handleKey(event) ||
      regions.handleKey(event) ||
      hold.keyDown(event) ||
      viewport.handleKey(event) ||
      compare.handleKey(event),
    onStageKeyUp: (event: { key: string }) => hold.keyUp(event),
  };
}

type CompareViewportOptions = {
  stageRef: RefObject<HTMLDivElement | null>;
  areaRef: RefObject<HTMLDivElement | null>;
  sizes: [Size | undefined, Size | undefined];
  marking: boolean;
  pinned: boolean;
  compare: ReturnType<typeof useReviewCompare>;
  carry: ReviewCarry | undefined;
  viewKey: string;
};

/**
 * The comparison's zoom and pan: sized to both images, fitted on entering or
 * leaving pinned Wipe (with the line at the carried split), remembered per
 * stage once both images are measured, and carrying a scrubbed pinned split.
 */
function useCompareViewport(options: CompareViewportOptions) {
  const { stageRef, areaRef, sizes, marking, pinned, compare } = options;
  const imageSize = unionSize(sizes[0], sizes[1]);
  const wipe = compare.state.wipe;
  const viewport = useReviewViewport(stageRef, imageSize, {
    areaRef,
    panEnabled: !marking,
    pinned,
    pinnedAt: wipe,
  });
  const fitView = viewport.fit;
  useEffect(() => {
    fitView();
  }, [pinned, fitView]);
  useRememberedView(viewport, options.carry, options.viewKey, measuredPairSize(sizes[0], sizes[1]));
  useCarryPinnedSplit({
    pinned,
    measured: viewport.stage.width > 0 && imageSize.width > 0,
    fraction: viewport.pinnedFraction,
    wipe,
    dispatch: compare.dispatch,
  });
  return viewport;
}

/** The Wipe divider: its value and setter, and when the handle shows the names. */
function useStageWipe(
  viewport: ReturnType<typeof useReviewViewport>,
  compare: ReturnType<typeof useReviewCompare>,
  pinned: boolean,
  wiping: boolean,
) {
  const value = pinned ? viewport.pinnedFraction : compare.state.wipe;
  const reveal = useWipeNamesReveal();
  const { view } = viewport;
  const wasPinned = useRef(pinned);
  const enteringPinned = pinned && !wasPinned.current;
  useEffect(() => {
    wasPinned.current = pinned;
  }, [pinned]);
  usePulseOnSplitMove(
    reveal,
    wiping && !enteringPinned,
    `${value}:${view.x}:${view.y}:${view.scale}`,
  );
  return {
    value,
    onChange: (next: number) =>
      pinned ? viewport.setPinnedFraction(next) : compare.dispatch({ type: "wipe", value: next }),
    reveal,
  };
}

export type CompareStageModel = ReturnType<typeof useCompareStage>;

/**
 * Scrubbing the pinned touch Wipe moves the images, not the reducer's wipe
 * value; copying the split back keeps it in the carried settings, so the next
 * item or question starts with the line where the operator left it.
 */
function useCarryPinnedSplit(args: {
  pinned: boolean;
  measured: boolean;
  fraction: number;
  wipe: number;
  dispatch: Dispatch<CompareAction>;
}) {
  const { pinned, measured, fraction, wipe, dispatch } = args;
  const wasPinned = useRef(false);
  useEffect(() => {
    if (!pinned) {
      wasPinned.current = false;
      return;
    }
    if (!wasPinned.current) {
      wasPinned.current = true;
      return;
    }
    if (measured && Math.abs(fraction - wipe) > 0.001) {
      dispatch({ type: "wipe", value: fraction });
    }
  }, [pinned, measured, fraction, wipe, dispatch]);
}

/** A pair keeps its own view; an operator comparison keeps one per segment. */
function stageViewKey(
  questionId: string,
  props: { pair?: boolean; compareImages: readonly { id: string }[] },
) {
  const ids = props.pair ? props.compareImages.map((image) => image.id).join("|") : "compare";
  return `${questionId}:${ids}`;
}
