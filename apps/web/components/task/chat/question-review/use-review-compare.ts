import { useCallback, useReducer } from "react";
import { compareReducer, initialCompareState } from "@/lib/question-review/compare";
import type { ReviewImage } from "@/lib/question-review/segments";

const KEY_ACTIONS = {
  t: { type: "flip" },
  s: { type: "swap" },
  "[": { type: "nudge", direction: -1 },
  "]": { type: "nudge", direction: 1 },
} as const;

export function useReviewCompare(images: readonly ReviewImage[]) {
  const [state, dispatch] = useReducer(
    compareReducer,
    images.map((image) => image.id),
    initialCompareState,
  );
  const handleKey = useCallback((event: { key: string }) => {
    const action = KEY_ACTIONS[event.key as keyof typeof KEY_ACTIONS];
    if (action) dispatch(action);
    return Boolean(action);
  }, []);
  return { state, dispatch, handleKey };
}

export type ReviewCompare = ReturnType<typeof useReviewCompare>;
