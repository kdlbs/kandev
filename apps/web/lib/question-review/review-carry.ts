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
  /** The view a stage was left at (Fit included), or else the last view when the image size matches. */
  recallView: (key: string, image: Size) => ReviewView | null;
  /** `null` records Fit. */
  rememberView: (key: string, image: Size, view: ReviewView | null) => void;
};

function sameSize(a: Size, b: Size): boolean {
  return a.width === b.width && a.height === b.height;
}

export function createReviewCarry(): ReviewCarry {
  let settings: CompareSettings | undefined;
  const views = new Map<string, ReviewView | null>();
  let last: { image: Size; view: ReviewView | null } | null = null;
  return {
    read: () => settings,
    write: (next) => {
      settings = next;
    },
    recallView: (key, image) => {
      if (views.has(key)) return views.get(key) ?? null;
      return last && sameSize(last.image, image) ? last.view : null;
    },
    rememberView: (key, image, view) => {
      views.set(key, view);
      last = { image, view };
    },
  };
}
