import { useCallback, useEffect, useRef, useState } from "react";
import type { ReviewSegment } from "@/lib/question-review/segments";
import type { ImageRequest } from "./review-image-request";
import type { ReviewAnnotationController } from "./use-review-annotations";

/** A region selection waiting for the stage that shows its image. */
export type RegionReveal = { imageId: string | null; consume: () => void };

/**
 * Turns each new region selection, and each Show image request from the answer
 * form, into one reveal: move to the item that shows the image, then let that
 * item's stage show it once. A request is never replayed when the operator
 * later navigates or a stage remounts, so it cannot override Back to gallery
 * or a new comparison.
 */
export function useRegionReveal(
  annotations: ReviewAnnotationController,
  segment: ReviewSegment,
  showImage: (imageId: string) => void,
  request?: ImageRequest | null,
): RegionReveal {
  const { selected, list } = annotations;
  const imageId =
    selected?.questionId === segment.questionId
      ? list(segment.questionId)[selected.index]?.image_id
      : undefined;
  const handled = useRef(selected);
  const [pending, setPending] = useState<string | null>(null);
  useEffect(() => {
    if (selected === handled.current) return;
    handled.current = selected;
    if (!imageId) return;
    showImage(imageId);
    setPending(imageId);
  }, [imageId, selected, showImage]);
  const handledRequest = useRef(request);
  useEffect(() => {
    if (!request || request === handledRequest.current) return;
    handledRequest.current = request;
    if (!segment.images.some((image) => image.id === request.imageId)) return;
    showImage(request.imageId);
    setPending(request.imageId);
  }, [request, segment.images, showImage]);
  const consume = useCallback(() => setPending(null), []);
  return { imageId: pending, consume };
}
