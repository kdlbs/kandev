import { useRef, useState } from "react";
import {
  centerRegion,
  moveRegion,
  rectFromPoints,
  resizeRegion,
  stageToImage,
  type RegionRect,
} from "@/lib/question-review/regions";
import type { Point, ReviewView, Size } from "@/lib/question-review/viewport";
import type { ReviewAnnotationController } from "./use-review-annotations";
import { pointIn } from "./use-review-viewport";

type PointerHandlers = {
  onPointerDown: (event: React.PointerEvent<HTMLElement>) => void;
  onPointerMove: (event: React.PointerEvent<HTMLElement>) => void;
  onPointerUp: (event: React.PointerEvent<HTMLElement>) => void;
  onPointerCancel: (event: React.PointerEvent<HTMLElement>) => void;
};

type Args = {
  /** True when annotate mode is on and the stage shows a single image. */
  active: boolean;
  questionId: string;
  imageId: string | undefined;
  image: Size;
  view: ReviewView;
  stage: Size;
  controller: ReviewAnnotationController;
  viewportHandlers: PointerHandlers;
};

const MIN_DRAG_PX = 5;

const ARROWS: Record<string, [number, number]> = {
  ArrowLeft: [-1, 0],
  ArrowRight: [1, 0],
  ArrowUp: [0, -1],
  ArrowDown: [0, 1],
};

/**
 * Draws regions with the primary pointer in annotate mode. Every pointer event
 * still reaches the viewport, so a second finger turns the gesture into a
 * pinch and cancels the region being drawn.
 */
export function useRegionDrawing(args: Args) {
  const start = useRef<{
    pointerId: number;
    point: Point;
    clientX: number;
    clientY: number;
  } | null>(null);
  const [draft, setDraft] = useState<RegionRect | null>(null);
  const ready = args.active && Boolean(args.imageId) && args.image.width > 0;
  const toImage = (event: React.PointerEvent<HTMLElement>) =>
    stageToImage(args.view, pointIn(event.currentTarget, event));
  const cancel = () => {
    start.current = null;
    setDraft(null);
  };

  const pointerHandlers: PointerHandlers = {
    onPointerDown: (event) => {
      args.viewportHandlers.onPointerDown(event);
      if (!ready || !event.isPrimary) return cancel();
      start.current = {
        pointerId: event.pointerId,
        point: toImage(event),
        clientX: event.clientX,
        clientY: event.clientY,
      };
    },
    onPointerMove: (event) => {
      args.viewportHandlers.onPointerMove(event);
      if (start.current?.pointerId !== event.pointerId) return;
      setDraft(rectFromPoints(start.current.point, toImage(event), args.image));
    },
    onPointerUp: (event) => {
      args.viewportHandlers.onPointerUp(event);
      const origin = start.current;
      if (origin?.pointerId !== event.pointerId || !args.imageId) return cancel();
      cancel();
      // A press without a real drag is a click, which may open an existing region.
      const dragged = Math.hypot(event.clientX - origin.clientX, event.clientY - origin.clientY);
      if (dragged < MIN_DRAG_PX) return;
      const rect = rectFromPoints(origin.point, toImage(event), args.image);
      if (rect) args.controller.add(args.questionId, args.imageId, rect, args.image);
    },
    onPointerCancel: (event) => {
      args.viewportHandlers.onPointerCancel(event);
      cancel();
    },
  };

  const handleKey = (event: { key: string; shiftKey: boolean; altKey: boolean }): boolean => {
    if (!ready || !args.imageId) return false;
    const { controller, questionId, image } = args;
    if (event.key === "n") {
      controller.add(questionId, args.imageId, centerRegion(args.view, args.stage, image), image);
      return true;
    }
    const selected = controller.selected;
    const region =
      selected?.questionId === questionId ? controller.list(questionId)[selected.index] : undefined;
    if (!selected || !region || region.image_id !== args.imageId) return false;
    if (event.key === "Enter") {
      controller.editComment(questionId, selected.index);
      return true;
    }
    if (event.key === "Delete" || event.key === "Backspace") {
      controller.remove(questionId, selected.index);
      return true;
    }
    const arrow = ARROWS[event.key];
    if (!arrow) return false;
    const step = event.shiftKey ? 10 : 1;
    const [dx, dy] = [arrow[0] * step, arrow[1] * step];
    const next = event.altKey
      ? resizeRegion(region, dx, dy, image)
      : moveRegion(region, dx, dy, image);
    controller.update(questionId, selected.index, next);
    return true;
  };

  return { pointerHandlers: ready ? pointerHandlers : args.viewportHandlers, handleKey, draft };
}
