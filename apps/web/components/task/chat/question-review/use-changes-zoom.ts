import { useCallback, useEffect, useState } from "react";
import type { CompareMode } from "@/lib/question-review/compare";
import { useReviewDiff, type ReviewDiffResult } from "./use-review-diff";

export const CHANGE_FLASH_MS = 1500;

type Bounds = NonNullable<ReviewDiffResult["bounds"]>;
export type ChangesStatus = "idle" | "computing" | "none" | "unavailable";

type Args = {
  mode: CompareMode;
  /** Identifies the image pair and tolerance; a new key forgets earlier requests. */
  pairKey: string;
  a: Blob | undefined;
  b: Blob | undefined;
  tolerance: number;
  zoomToRect: (bounds: Bounds) => void;
};

/**
 * Zoom to changes in any mode. Difference computes the changed pixels anyway;
 * other modes compute them only when asked, and keep the result for the pair.
 */
export function useChangesZoom({ mode, pairKey, a, b, tolerance, zoomToRect }: Args) {
  const [requestedKey, setRequestedKey] = useState<string | null>(null);
  const [pendingKey, setPendingKey] = useState<string | null>(null);
  const [flash, setFlash] = useState<Bounds | null>(null);
  const requested = requestedKey === pairKey;
  const diff = useReviewDiff(mode === "diff" || requested, a, b, tolerance);
  const bounds = diff.status === "ready" ? diff.result.bounds : null;

  const zoomTo = useCallback(
    (target: Bounds) => {
      zoomToRect(target);
      if (mode !== "diff") setFlash(target);
    },
    [mode, zoomToRect],
  );

  useEffect(() => {
    if (pendingKey !== pairKey || diff.status === "computing" || diff.status === "idle") return;
    setPendingKey(null);
    if (bounds) zoomTo(bounds);
  }, [bounds, diff.status, pairKey, pendingKey, zoomTo]);

  useEffect(() => {
    if (!flash) return;
    const timer = setTimeout(() => setFlash(null), CHANGE_FLASH_MS);
    return () => clearTimeout(timer);
  }, [flash]);

  const trigger = useCallback(() => {
    if (bounds) return zoomTo(bounds);
    setRequestedKey(pairKey);
    setPendingKey(pairKey);
  }, [bounds, pairKey, zoomTo]);

  return {
    diff,
    trigger,
    status: changesStatus(
      diff.status,
      bounds,
      mode === "diff" || requested,
      pendingKey === pairKey,
    ),
    outline: mode === "diff" ? bounds : flash,
  };
}

function changesStatus(
  status: ReturnType<typeof useReviewDiff>["status"],
  bounds: Bounds | null,
  active: boolean,
  pending: boolean,
): ChangesStatus {
  if (pending || (active && status === "computing")) return "computing";
  if (!active) return "idle";
  if (status === "unavailable") return "unavailable";
  if (status === "ready" && !bounds) return "none";
  return "idle";
}
