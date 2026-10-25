"use client";

import { useEffect, useState } from "react";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import type { AttentionTask } from "@/lib/coordinator/attention";

export type TargetTask = {
  /** The task identifier, or the task id when the task is not known. */
  label: string;
  /** True when the task is known, so the label can link to it. */
  known: boolean;
};

/**
 * The target task of a resume, message or move card. With a task map (the
 * Needs-you page) the map decides; without one (the compact chat card) the
 * task is read once. An unknown task shows its id and no link.
 */
export function useTargetTask(
  taskId: string,
  openTasksById: Map<string, AttentionTask> | undefined,
): TargetTask {
  const mapped = openTasksById?.get(taskId);
  const [fetched, setFetched] = useState<{ taskId: string; identifier: string } | null>(null);
  const needsRead = openTasksById === undefined;

  useEffect(() => {
    if (!needsRead) return;
    let cancelled = false;
    fetchTask(taskId)
      .then((task) => {
        if (!cancelled && task.identifier) setFetched({ taskId, identifier: task.identifier });
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [needsRead, taskId]);

  if (mapped) return { label: mapped.identifier || mapped.id, known: true };
  if (fetched && fetched.taskId === taskId) return { label: fetched.identifier, known: true };
  return { label: taskId, known: false };
}
