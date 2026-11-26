import { useEffect, useRef } from "react";
import type { ReviewCarry } from "@/lib/question-review/review-carry";
import type { ReviewView, Size } from "@/lib/question-review/viewport";

type Viewport = { view: ReviewView; atFit: boolean; restore: (view: ReviewView) => void };

/**
 * Brings a stage back to the zoom and pan it was left at, or to the last view
 * when the image has the same size, and records the view as it changes. The
 * recall waits until the image size is known, and happens once per mount.
 */
export function useRememberedView(
  viewport: Viewport,
  carry: ReviewCarry | undefined,
  key: string,
  image: Size,
) {
  const recalled = useRef(false);
  const restoring = useRef(false);
  const { width, height } = image;
  const measured = width > 0 && height > 0;
  const { view, atFit, restore } = viewport;
  useEffect(() => {
    if (recalled.current || !measured || !carry) return;
    recalled.current = true;
    const kept = carry.recallView(key, { width, height });
    if (kept) {
      restoring.current = true;
      restore(kept);
    }
  }, [measured, carry, key, width, height, restore]);
  useEffect(() => {
    if (!recalled.current || !carry || !measured) return;
    if (restoring.current) {
      restoring.current = false;
      return;
    }
    carry.rememberView(key, { width, height }, atFit ? null : view);
  }, [carry, key, width, height, measured, atFit, view]);
}
