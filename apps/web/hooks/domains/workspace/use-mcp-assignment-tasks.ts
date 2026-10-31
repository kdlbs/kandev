"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { listTasksByWorkspace } from "@/lib/api/domains/kanban-api";
import type { Task } from "@/lib/types/http";

type TaskPage = {
  key: string;
  tasks: Task[];
  total: number;
  loading: boolean;
  failed: boolean;
};

export function useMCPAssignmentTasks(
  workspaceId: string,
  { active, page, query }: { active: boolean; page: number; query: string },
) {
  const [attempt, setAttempt] = useState(0);
  const [result, setResult] = useState<TaskPage | null>(null);
  const generation = useRef(0);
  const key = JSON.stringify([workspaceId, active, page, query, attempt]);
  const currentKey = useRef(key);
  currentKey.current = key;

  useEffect(() => {
    if (!active || !workspaceId) return;
    const request = ++generation.current;
    const abort = new AbortController();
    const isCurrent = () =>
      !abort.signal.aborted && generation.current === request && currentKey.current === key;
    setResult({ key, tasks: [], total: 0, loading: true, failed: false });
    void listTasksByWorkspace(
      workspaceId,
      { page, pageSize: 25, query, includeArchived: false },
      { init: { signal: abort.signal } },
    ).then(
      ({ tasks, total }) => {
        if (!isCurrent()) return;
        setResult({
          key,
          tasks: tasks.filter(
            (task) =>
              task.workspace_id === workspaceId && !task.archived_at && !task.workspace_orphaned,
          ),
          total,
          loading: false,
          failed: false,
        });
      },
      () => {
        if (isCurrent()) setResult({ key, tasks: [], total: 0, loading: false, failed: true });
      },
    );
    return () => abort.abort();
  }, [workspaceId, active, page, query, key]);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);
  const visible = active && result?.key === key ? result : null;
  return {
    tasks: visible?.tasks ?? [],
    total: visible?.total ?? 0,
    loading: active && !!workspaceId && (visible?.loading ?? true),
    failed: visible?.failed ?? false,
    reload,
  };
}
