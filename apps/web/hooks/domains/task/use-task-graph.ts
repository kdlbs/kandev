import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  getTaskGraph,
  type TaskGraphMode,
  type TaskGraphResponse,
} from "@/lib/api/domains/task-graph-api";
import { buildTaskGraphModel, type TaskGraphModel } from "@/lib/task-graph/model";

const REFRESH_DEBOUNCE_MS = 500;

type SignatureTask = {
  id: string;
  title?: string;
  state?: string;
  parentTaskId?: string | null;
  isArchived?: boolean;
  dependsOn?: Array<{ id: string }>;
  blocks?: Array<{ id: string }>;
  related?: Array<{ id: string }>;
};

/**
 * A primitive digest of the store rows the graph displays, plus new children
 * of displayed tasks, so the graph refetches when any of them change.
 */
export function taskGraphSignature(
  tasks: SignatureTask[],
  snapshots: Record<string, { tasks: SignatureTask[] }>,
  watched: Set<string>,
): string {
  const parts = new Map<string, string>();
  const visit = (t: SignatureTask) => {
    if (parts.has(t.id)) return;
    if (!watched.has(t.id) && !(t.parentTaskId && watched.has(t.parentTaskId))) return;
    const deps = (t.dependsOn ?? []).map((d) => d.id).join(",");
    const blocks = (t.blocks ?? []).map((d) => d.id).join(",");
    const related = (t.related ?? [])
      .map((d) => d.id)
      .sort()
      .join(",");
    parts.set(
      t.id,
      [
        t.id,
        t.title,
        t.state,
        t.parentTaskId ?? "",
        t.isArchived ? 1 : 0,
        deps,
        blocks,
        related,
      ].join("|"),
    );
  };
  tasks.forEach(visit);
  for (const snapshot of Object.values(snapshots)) snapshot.tasks.forEach(visit);
  return [...parts.values()].sort().join("\n");
}

function watchedIds(data: TaskGraphResponse | null): Set<string> {
  if (!data) return new Set();
  const ids = new Set(data.nodes.map((n) => n.id));
  ids.add(data.focus_task_id);
  if (data.root) ids.add(data.root.id);
  return ids;
}

export type UseTaskGraphResult = {
  model: TaskGraphModel | null;
  loading: boolean;
  error: boolean;
  setMode: (mode: TaskGraphMode) => void;
  retry: () => void;
};

export function useTaskGraph(taskId: string, open: boolean): UseTaskGraphResult {
  const [requestedMode, setRequestedMode] = useState<TaskGraphMode | undefined>(undefined);
  const [data, setData] = useState<TaskGraphResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const generation = useRef(0);

  const load = useCallback(
    (mode: TaskGraphMode | undefined) => {
      const current = ++generation.current;
      setLoading(true);
      getTaskGraph(taskId, mode)
        .then((response) => {
          if (current !== generation.current) return;
          setData(response);
          setError(false);
        })
        .catch(() => {
          if (current === generation.current) setError(true);
        })
        .finally(() => {
          if (current === generation.current) setLoading(false);
        });
    },
    [taskId],
  );

  useEffect(() => {
    if (!open) return;
    load(requestedMode);
    return () => {
      generation.current++;
    };
  }, [open, requestedMode, load]);

  useEffect(() => {
    if (!open) {
      setData(null);
      setRequestedMode(undefined);
    }
  }, [open]);

  const watched = useMemo(() => watchedIds(data), [data]);
  const signature = useAppStore((state) =>
    taskGraphSignature(
      state.kanban.tasks as SignatureTask[],
      state.kanbanMulti.snapshots as Record<string, { tasks: SignatureTask[] }>,
      watched,
    ),
  );
  const baseline = useRef<{ watched: Set<string>; signature: string } | null>(null);
  const loadedMode = data?.mode;

  useEffect(() => {
    if (!open || !loadedMode) {
      baseline.current = null;
      return;
    }
    // A new response re-baselines instead of refetching for its own nodes.
    const previous = baseline.current;
    baseline.current = { watched, signature };
    if (!previous || previous.watched !== watched || previous.signature === signature) return;
    const timer = setTimeout(() => load(loadedMode), REFRESH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [open, loadedMode, watched, signature, load]);

  const model = useMemo(() => (data ? buildTaskGraphModel(data) : null), [data]);
  const retry = useCallback(
    () => load(requestedMode ?? loadedMode),
    [load, requestedMode, loadedMode],
  );

  return { model, loading, error, setMode: setRequestedMode, retry };
}
