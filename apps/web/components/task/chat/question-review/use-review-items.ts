import { useCallback, useState } from "react";
import { itemIndexForImage, type ReviewSegment } from "@/lib/question-review/segments";

export type OperatorCompare = { aId: string; bId: string };

/**
 * Item navigation for one segment: the current pair, single, or gallery image,
 * and an operator comparison started from the gallery. The stage that owns this
 * hook is keyed by segment, so a new segment starts at its first item.
 */
export function useReviewItems(segment: ReviewSegment) {
  const { items, images } = segment;
  const count = items.length;
  const [rawIndex, setIndex] = useState(0);
  const [compare, setCompare] = useState<OperatorCompare | null>(null);
  const index = count === 0 ? 0 : Math.min(rawIndex, count - 1);
  const item = items[index];

  const step = useCallback(
    (direction: 1 | -1) =>
      setIndex((current) => Math.min(Math.max(current + direction, 0), Math.max(count - 1, 0))),
    [count],
  );
  const canCompare = !segment.grouped && images.length > 1;
  const startCompare = useCallback(() => {
    if (!canCompare || item?.kind !== "gallery") return;
    const position = images.findIndex((image) => image.id === item.image.id);
    const partner = images[position + 1] ?? images[position - 1];
    if (partner) setCompare({ aId: item.image.id, bId: partner.id });
  }, [canCompare, images, item]);
  const backToGallery = useCallback(
    (aId: string) => {
      setCompare(null);
      const next = itemIndexForImage(items, aId);
      if (next >= 0) setIndex(next);
    },
    [items],
  );
  const showImage = useCallback(
    (imageId: string) => {
      // An operator comparison spans every segment image; its stage shows the region.
      if (compare) return;
      const next = itemIndexForImage(items, imageId);
      if (next >= 0) setIndex(next);
    },
    [compare, items],
  );
  const handleKey = useCallback(
    (event: { key: string }) => {
      if (compare || count < 2) return false;
      if (event.key === "PageDown") step(1);
      else if (event.key === "PageUp") step(-1);
      else return false;
      return true;
    },
    [compare, count, step],
  );

  return {
    index,
    item,
    count,
    canPrev: index > 0,
    canNext: index < count - 1,
    step,
    compare,
    canCompare,
    startCompare,
    backToGallery,
    showImage,
    handleKey,
  };
}

export type ReviewItems = ReturnType<typeof useReviewItems>;
