import { useEffect, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getTaskDependencies } from "@/lib/api/domains/task-dependencies-api";
import type { TaskDependencyRef } from "@/lib/state/slices/kanban/types";

export type DependencyChipData = {
  blocked: boolean;
  blockedReason?: string;
  dependsOn: TaskDependencyRef[];
  blocks: TaskDependencyRef[];
  related: TaskDependencyRef[];
};

/**
 * useTaskDependencies reads the derived dependency projection for a task from
 * the store. Returns null when the task is unknown or has no edges in either
 * direction, which is also the chip's "render nothing" signal.
 *
 * The selector returns the task itself — a stable reference — and the derived
 * shape is built in a memo afterwards. Returning a freshly-built object FROM the
 * selector compares unequal under Object.is on every store read, which loops
 * until React bails out and renders nothing at all.
 */
export function useTaskDependencies(taskId: string | null): DependencyChipData | null {
  const fromStore = useTaskDependenciesFromStore(taskId);
  const storeAnswered = fromStore !== undefined;
  const fetched = useFetchedTaskDependencies(taskId, storeAnswered);
  // `??` would be wrong here: the store's `null` is a real answer ("this task
  // has no edges"), and falling through on it would keep showing the previously
  // fetched task's dependencies after an edge is removed or after switching to
  // a task the store knows is edge-free. Only `undefined` means "ask the server".
  return storeAnswered ? fromStore : fetched;
}

/**
 * Fetches the projection when the store copy has none.
 *
 * `undefined` from the store means "this copy carries no dependency fields" (a
 * lightweight task.updated overwrote them, or landed before hydration); `null`
 * means "read it, there are no edges". Only the former needs a fetch.
 */
function useFetchedTaskDependencies(
  taskId: string | null,
  storeAnswered: boolean,
): DependencyChipData | null {
  const [data, setData] = useState<DependencyChipData | null>(null);
  useEffect(() => {
    if (!taskId || storeAnswered) {
      // Drop anything fetched for a previous task/state so it can never be
      // rendered again once the store can answer.
      setData(null);
      return;
    }
    let cancelled = false;
    void getTaskDependencies(taskId)
      .then((task) => {
        if (cancelled) return;
        const dependsOn = (task.depends_on ?? []) as TaskDependencyRef[];
        const blocks = (task.blocks ?? []) as TaskDependencyRef[];
        const related = (task.related ?? []) as TaskDependencyRef[];
        setData(
          dependsOn.length === 0 && blocks.length === 0 && related.length === 0
            ? null
            : {
                blocked: task.blocked ?? false,
                blockedReason: task.blocked_reason,
                dependsOn,
                blocks,
                related,
              },
        );
      })
      .catch(() => {
        // A failed read hides the chip rather than showing a wrong state.
        if (!cancelled) setData(null);
      });
    return () => {
      cancelled = true;
    };
  }, [taskId, storeAnswered]);
  return data;
}

/**
 * Store fast path. Returns `undefined` when the store copy carries no dependency
 * projection at all, so the caller knows to fetch rather than render nothing.
 */
function useTaskDependenciesFromStore(
  taskId: string | null,
): DependencyChipData | null | undefined {
  const task = useAppStore((state) => {
    if (!taskId) return undefined;
    // Nil-safe on both slices: this renders inside a task panel that can mount
    // before board hydration completes.
    const fromBoard = state.kanban?.tasks?.find((candidate) => candidate.id === taskId);
    if (fromBoard) return fromBoard;
    for (const snapshot of Object.values(state.kanbanMulti?.snapshots ?? {})) {
      const hit = snapshot.tasks?.find((candidate) => candidate.id === taskId);
      if (hit) return hit;
    }
    return undefined;
  });
  return useMemo(() => {
    if (!task) return undefined;
    // No projection on this copy: the caller must ask the server.
    if (task.dependsOn === undefined && task.blocks === undefined && task.related === undefined)
      return undefined;
    const dependsOn = task.dependsOn ?? [];
    const blocks = task.blocks ?? [];
    const related = task.related ?? [];
    if (dependsOn.length === 0 && blocks.length === 0 && related.length === 0) return null;
    return {
      blocked: task.blocked ?? false,
      blockedReason: task.blockedReason,
      dependsOn,
      blocks,
      related,
    };
  }, [task]);
}
