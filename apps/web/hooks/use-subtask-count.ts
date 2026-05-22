import { useEffect, useState } from "react";
import { getSubtaskCount } from "@/lib/api";

// useSubtaskCount fetches the subtask count for an archive / delete
// confirmation dialog when it opens. Returns 0 while the request for
// the current set of ids is still in flight (or fails, or no ids were
// supplied) — in all of those cases the dialog's cascade checkbox
// stays hidden.
//
// The stored count is keyed by a stable string of the requested ids
// AND only returned while the dialog reports `open=true`. Closing the
// dialog clears the cache, so reopening — even for the same task —
// shows 0 until the fresh fetch lands. This avoids a stale count from
// a previous opening flashing before the new request resolves and
// stops the bulk toolbar's per-render `[...selectedIds]` from fanning
// out a new Promise.all on every render.
export function useSubtaskCount(open: boolean, taskId?: string, taskIds?: string[]): number {
  const idsKey = taskIds?.join(",") ?? taskId ?? "";
  const [result, setResult] = useState<{ key: string; total: number }>({
    key: "",
    total: 0,
  });
  useEffect(() => {
    if (!open) {
      // Clear so the next open starts from a known-empty state —
      // this is what gates stale counts from leaking across separate
      // dialog openings for the same id.
      setResult({ key: "", total: 0 });
      return;
    }
    if (!idsKey) return;
    const ids = taskIds ?? (taskId ? [taskId] : []);
    let cancelled = false;
    // Per-id .catch already maps every failure to { count: 0 }, so
    // Promise.all never rejects — no outer .catch needed.
    Promise.all(ids.map((id) => getSubtaskCount(id).catch(() => ({ count: 0 })))).then(
      (results) => {
        if (cancelled) return;
        setResult({ key: idsKey, total: results.reduce((sum, r) => sum + r.count, 0) });
      },
    );
    return () => {
      cancelled = true;
    };
    // taskId / taskIds intentionally excluded — idsKey is their stable summary.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, idsKey]);
  return open && result.key === idsKey ? result.total : 0;
}
