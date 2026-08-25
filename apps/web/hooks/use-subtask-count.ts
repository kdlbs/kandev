import { useEffect, useState } from "react";
import { getSubtaskCount } from "@/lib/api";

export type SubtaskCountStatus = "idle" | "loading" | "resolved" | "error";

export type SubtaskCountResult = {
  status: SubtaskCountStatus;
  total: number;
};

// useSubtaskCount fetches the subtask count for an archive / delete
// confirmation dialog when it opens. Returns 0 while the request for
// the current set of ids is still in flight or fails — callers that need
// to distinguish those safe states use useSubtaskCountState below.
//
// The stored count is keyed by a stable string of the requested ids
// AND only returned while the dialog reports `open=true`. Closing the
// dialog clears the cache, so reopening — even for the same task —
// shows 0 until the fresh fetch lands. This avoids a stale count from
// a previous opening flashing before the new request resolves and
// stops the bulk toolbar's per-render `[...selectedIds]` from fanning
// out a new Promise.all on every render.
export function useSubtaskCountState(
  open: boolean,
  taskId?: string,
  taskIds?: string[],
): SubtaskCountResult {
  const idsKey = taskIds?.join(",") ?? taskId ?? "";
  const [result, setResult] = useState<{ key: string } & SubtaskCountResult>({
    key: "",
    status: "idle",
    total: 0,
  });
  useEffect(() => {
    if (!open) {
      // Clear so the next open starts from a known-empty state —
      // this is what gates stale counts from leaking across separate
      // dialog openings for the same id.
      setResult({ key: "", status: "idle", total: 0 });
      return;
    }
    if (!idsKey) {
      setResult({ key: idsKey, status: "error", total: 0 });
      return;
    }
    const ids = taskIds ?? (taskId ? [taskId] : []);
    let cancelled = false;
    setResult({ key: idsKey, status: "loading", total: 0 });
    Promise.all(ids.map((id) => getSubtaskCount(id)))
      .then((results) => {
        if (cancelled) return;
        setResult({
          key: idsKey,
          status: "resolved",
          total: results.reduce((sum, r) => sum + r.count, 0),
        });
      })
      .catch(() => {
        if (!cancelled) setResult({ key: idsKey, status: "error", total: 0 });
      });
    return () => {
      cancelled = true;
    };
    // taskId / taskIds intentionally excluded — idsKey is their stable summary.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, idsKey]);
  if (!open || result.key !== idsKey) return { status: "idle", total: 0 };
  return { status: result.status, total: result.total };
}

export function useSubtaskCount(open: boolean, taskId?: string, taskIds?: string[]): number {
  const result = useSubtaskCountState(open, taskId, taskIds);
  return result.status === "resolved" ? result.total : 0;
}
