import { useCallback, useRef, type KeyboardEvent, type PointerEvent, type RefObject } from "react";
import type { ReviewFile } from "./types";
import { useReviewScrollAnchor } from "./use-review-scroll-anchor";

const SCROLL_KEYS = new Set(["ArrowDown", "ArrowUp", "PageDown", "PageUp", "Home", "End", " "]);

export function useReviewDiffScrollInteraction(args: {
  rootRef: RefObject<HTMLDivElement | null>;
  files: ReviewFile[];
  sessionId: string;
  sourceKey?: string;
}) {
  const suppressAutoMarkRef = useRef(true);
  const { captureOnScroll, handleUserInput } = useReviewScrollAnchor({
    ...args,
    suppressAutoMark: suppressAutoMarkRef,
  });
  const allowAutoMark = useCallback(() => {
    suppressAutoMarkRef.current = false;
    handleUserInput();
  }, [handleUserInput]);
  const handlePointerDown = useCallback(
    (event: PointerEvent<HTMLDivElement>) => {
      if (event.target === event.currentTarget) suppressAutoMarkRef.current = false;
      handleUserInput();
    },
    [handleUserInput],
  );
  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLDivElement>) => {
      if (!SCROLL_KEYS.has(event.key)) return;
      suppressAutoMarkRef.current = false;
      handleUserInput();
    },
    [handleUserInput],
  );
  return {
    suppressAutoMarkRef,
    captureOnScroll,
    allowAutoMark,
    handlePointerDown,
    handleKeyDown,
  };
}
