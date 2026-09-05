"use client";

import { useEffect, useRef, useState } from "react";
import { listTaskCanvases, type Canvas } from "@/lib/api/domains/canvas-api";
import type { Task } from "@/lib/types/http";
import { useCanvasLifecycleRevision } from "@/lib/canvas-lifecycle";

const EMPTY_CANVASES: Canvas[] = [];

/** Loads the canvases that can be opened from a task's mobile Panels picker. */
export function useTaskCanvases(
  taskId: string | null | undefined,
  workspaceId: string | null | undefined,
  enabled = true,
): Canvas[] {
  const [loaded, setLoaded] = useState<{ key: string | null; canvases: Canvas[] }>({
    key: null,
    canvases: EMPTY_CANVASES,
  });
  const requestRef = useRef(0);
  const lifecycleRevision = useCanvasLifecycleRevision();

  useEffect(() => {
    const requestId = ++requestRef.current;
    if (!enabled || !taskId || !workspaceId) {
      setLoaded({ key: null, canvases: EMPTY_CANVASES });
      return;
    }

    const key = `${workspaceId}\u0000${taskId}`;
    setLoaded((current) => (current.key === key ? current : { key, canvases: EMPTY_CANVASES }));
    listTaskCanvases(taskId, { workspaceId, cache: "no-store" })
      .then((response) => {
        if (requestRef.current !== requestId) return;
        setLoaded({ key, canvases: response?.canvases ?? EMPTY_CANVASES });
      })
      .catch(() => {
        if (requestRef.current === requestId) setLoaded({ key, canvases: EMPTY_CANVASES });
      });

    return () => {
      if (requestRef.current === requestId) requestRef.current += 1;
    };
  }, [enabled, lifecycleRevision, taskId, workspaceId]);

  const key = taskId && workspaceId ? `${workspaceId}\u0000${taskId}` : null;
  return loaded.key === key ? loaded.canvases : EMPTY_CANVASES;
}

export function useTaskCanvasesForTask(
  task: Pick<Task, "id" | "workspace_id"> | null | undefined,
  isMobile: boolean,
  canvasesEnabled: boolean,
): Canvas[] {
  return useTaskCanvases(task?.id ?? null, task?.workspace_id ?? null, isMobile && canvasesEnabled);
}
