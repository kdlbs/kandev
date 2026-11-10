import { useCallback, useEffect, useMemo, useReducer } from "react";
import {
  candidatePosition,
  compareReducer,
  initialCompareState,
  type CompareSettings,
} from "@/lib/question-review/compare";
import type { ReviewImage } from "@/lib/question-review/segments";

/**
 * Comparison state for one segment. It starts from the previous segment's mode
 * and slider values, and reports changes so the next segment can start from them.
 */
export function useReviewCompare(
  images: readonly ReviewImage[],
  settings?: CompareSettings,
  onSettingsChange?: (settings: CompareSettings) => void,
) {
  const ids = useMemo(() => images.map((image) => image.id), [images]);
  const [state, dispatch] = useReducer(compareReducer, undefined, () =>
    initialCompareState(ids, settings),
  );
  const { mode, wipe, opacity, tolerance } = state;
  // Only the carried-over values are reported, never the image choices.
  useEffect(() => {
    onSettingsChange?.({ mode, wipe, opacity, tolerance });
  }, [mode, wipe, opacity, tolerance, onSettingsChange]);

  const stepping = mode === "toggle" || mode === "side";
  const handleKey = useCallback(
    (event: { key: string }) => {
      if (event.key === "t") dispatch({ type: "flip" });
      else if (event.key === "s") dispatch({ type: "swap" });
      else if (event.key === "[" || event.key === "]") {
        const direction = event.key === "]" ? 1 : -1;
        dispatch(stepping ? { type: "step", direction, ids } : { type: "nudge", direction });
      } else return false;
      return true;
    },
    [ids, stepping],
  );
  const step = useCallback(
    (direction: 1 | -1) => dispatch({ type: "step", direction, ids }),
    [ids],
  );
  return { state, dispatch, handleKey, step, ids, position: candidatePosition(state, ids) };
}

export type ReviewCompare = ReturnType<typeof useReviewCompare>;
