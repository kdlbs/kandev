import { useEffect, useState } from "react";
import { getSubtaskCount } from "@/lib/api";

// useSubtaskCount fetches the subtask count for an archive / delete
// confirmation dialog when it opens. Returns 0 while the request for
// the current set of ids is still in flight (or fails, or no ids were
// supplied) — in all of those cases the dialog's cascade checkbox
// stays hidden.
//
// The hook keys the stored count by a stable string of the requested
// ids. When the caller passes a different task (e.g. the dialog is
// closed and reopened for another row) the stale count is no longer
// returned, preventing the "Also archive 3 subtasks" label from
// flashing for a task that actually has 0. This also lets us depend on
// the joined string instead of the taskIds array — unstable array
// references (the multi-select toolbar spreads `selectedIds` on every
// render) no longer fan out a new Promise.all on every render.
export function useSubtaskCount(open: boolean, taskId?: string, taskIds?: string[]): number {
  const idsKey = taskIds?.join(",") ?? taskId ?? "";
  const [{ key, total }, setResult] = useState<{ key: string; total: number }>({
    key: "",
    total: 0,
  });
  useEffect(() => {
    if (!open || !idsKey) return;
    const ids = taskIds ?? (taskId ? [taskId] : []);
    let cancelled = false;
    Promise.all(ids.map((id) => getSubtaskCount(id).catch(() => ({ count: 0 }))))
      .then((results) => {
        if (cancelled) return;
        setResult({ key: idsKey, total: results.reduce((sum, r) => sum + r.count, 0) });
      })
      .catch(() => {
        // swallow — leaves prior result untouched; the stale-key gate
        // below still suppresses any leftover total.
      });
    return () => {
      cancelled = true;
    };
    // taskId / taskIds intentionally excluded — idsKey is their stable summary.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, idsKey]);
  return open && key === idsKey ? total : 0;
}
