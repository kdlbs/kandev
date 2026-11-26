import type { CompareSettings } from "./compare";
import type { ReviewView, Size } from "./viewport";

/**
 * What one review session carries from stage to stage: the comparison
 * settings, and each stage's zoom and pan. A holder, not state: stages read it
 * when they mount and write as values change, so dragging or zooming does not
 * re-render whatever owns it.
 */
export type ReviewCarry = {
  read: () => CompareSettings | undefined;
  write: (settings: CompareSettings) => void;
  /** The view a stage was left at, or else the last view when the image size matches. */
  recallView: (key: string, image: Size) => ReviewView | null;
  /** `null` records Fit. */
  rememberView: (key: string, image: Size, view: ReviewView | null) => void;
};

function sameSize(a: Size, b: Size): boolean {
  return a.width === b.width && a.height === b.height;
}

export function createReviewCarry(): ReviewCarry {
  let settings: CompareSettings | undefined;
  const views = new Map<string, ReviewView>();
  let last: { image: Size; view: ReviewView | null } | null = null;
  return {
    read: () => settings,
    write: (next) => {
      settings = next;
    },
    recallView: (key, image) => {
      const kept = views.get(key);
      if (kept) return kept;
      return last && sameSize(last.image, image) ? last.view : null;
    },
    rememberView: (key, image, view) => {
      if (view) views.set(key, view);
      else views.delete(key);
      last = { image, view };
    },
  };
}
