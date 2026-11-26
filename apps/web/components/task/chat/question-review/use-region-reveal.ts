import { useCallback, useEffect, useRef, useState } from "react";
import type { ReviewSegment } from "@/lib/question-review/segments";
import type { ReviewAnnotationController } from "./use-review-annotations";

/** A region selection waiting for the stage that shows its image. */
export type RegionReveal = { imageId: string | null; consume: () => void };

/**
 * Turns each new region selection into one reveal: move to the item that shows
 * the region's image, then let that item's stage show the image once. A
 * selection is never replayed when the operator later navigates or a stage
 * remounts, so it cannot override Back to gallery or a new comparison.
 */
export function useRegionReveal(
  annotations: ReviewAnnotationController,
  segment: ReviewSegment,
  showImage: (imageId: string) => void,
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
  const consume = useCallback(() => setPending(null), []);
  return { imageId: pending, consume };
}
