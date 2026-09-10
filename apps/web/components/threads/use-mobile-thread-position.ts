import { useEffect, useState, type RefObject } from "react";
import { nearestTaskId } from "./thread-viewport-geometry";

/** Phone orientation follows scroll geometry, independently of detail loading. */
export function useMobileThreadPosition({
  enabled,
  boardRef,
  elementsRef,
  orderedIds,
  fallbackTaskId,
}: {
  enabled: boolean;
  boardRef: RefObject<HTMLDivElement | null>;
  elementsRef: RefObject<Map<string, HTMLElement>>;
  orderedIds: readonly string[];
  fallbackTaskId: string | null;
}): string | null {
  const [taskId, setTaskId] = useState<string | null>(null);
  const idsKey = orderedIds.join("\u0000");

  useEffect(() => {
    const board = boardRef.current;
    if (!enabled || !board) return;
    const ids = idsKey ? idsKey.split("\u0000") : [];
    let frame: number | null = null;
    let disposed = false;
    const measure = () => {
      frame = null;
      if (disposed) return;
      const rect = board.getBoundingClientRect();
      const next =
        rect.right > rect.left ? nearestTaskId(ids, board, elementsRef.current) : fallbackTaskId;
      setTaskId((previous) => (previous === next ? previous : next));
    };
    const schedule = () => {
      if (!disposed && frame === null) frame = requestAnimationFrame(measure);
    };
    board.addEventListener("scroll", schedule, { passive: true });
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(schedule);
    observer?.observe(board);
    measure();
    return () => {
      disposed = true;
      board.removeEventListener("scroll", schedule);
      observer?.disconnect();
      if (frame !== null) cancelAnimationFrame(frame);
    };
  }, [enabled, boardRef, elementsRef, idsKey, fallbackTaskId]);

  if (!enabled) return null;
  return taskId && orderedIds.includes(taskId) ? taskId : fallbackTaskId;
}
