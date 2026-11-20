"use client";

import { createContext, useContext, type ReactNode } from "react";
import { createPortal } from "react-dom";

export type ReviewLayout = {
  /** Phone composition: top controls, image area, scrolling form, thumb-zone bar. */
  phone: boolean;
  /** Touch-first input: phones and coarse pointers. Wipe pins its line here. */
  touch: boolean;
  /** The fixed bottom bar element, present only in the phone composition. */
  bottomBar: HTMLElement | null;
};

const ReviewLayoutContext = createContext<ReviewLayout>({
  phone: false,
  touch: false,
  bottomBar: null,
});

export const ReviewLayoutProvider = ReviewLayoutContext.Provider;

export function useReviewLayout(): ReviewLayout {
  return useContext(ReviewLayoutContext);
}

/** Renders frequent stage actions into the phone's fixed bottom bar. */
export function ReviewBottomBar({ children }: { children: ReactNode }) {
  const { phone, bottomBar } = useReviewLayout();
  if (!phone || !bottomBar) return null;
  return createPortal(
    <div className="flex w-full items-center gap-2" data-testid="question-review-bottom-actions">
      {children}
    </div>,
    bottomBar,
  );
}
