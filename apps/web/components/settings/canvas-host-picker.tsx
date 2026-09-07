"use client";

import { useEffect, useRef, useState } from "react";
import { listTaskCanvases, listWorkspaceCanvases, type Canvas } from "@/lib/api/domains/canvas-api";
import { useCanvasLifecycleRevision } from "@/lib/canvas-lifecycle";

const EMPTY_CANVASES: Canvas[] = [];

/** Loads the canvases that share the focused host's task or workspace scope. */
export function useCanvasHostCanvases(canvas: Canvas | null): Canvas[] {
  const [loaded, setLoaded] = useState<{ key: string | null; canvases: Canvas[] }>({
    key: null,
    canvases: EMPTY_CANVASES,
  });
  const requestRef = useRef(0);
  const lifecycleRevision = useCanvasLifecycleRevision();
  const scopeKey = canvas
    ? `${canvas.scope_kind}\u0000${canvas.workspace_id}\u0000${canvas.task_id ?? ""}`
    : null;
  const isTaskCanvas = canvas?.scope_kind === "task";

  useEffect(() => {
    const requestId = ++requestRef.current;
    if (!canvas || !scopeKey) {
      setLoaded({ key: null, canvases: EMPTY_CANVASES });
      return;
    }
    if (isTaskCanvas && !canvas.task_id) {
      setLoaded({ key: scopeKey, canvases: EMPTY_CANVASES });
      return;
    }

    setLoaded((current) =>
      current.key === scopeKey ? current : { key: scopeKey, canvases: EMPTY_CANVASES },
    );
    const request = isTaskCanvas
      ? listTaskCanvases(canvas.task_id!, {
          workspaceId: canvas.workspace_id,
          cache: "no-store",
        })
      : listWorkspaceCanvases(canvas.workspace_id, { cache: "no-store" });
    request
      .then((response) => {
        if (requestRef.current !== requestId) return;
        const canvases = (response?.canvases ?? []).filter(
          (candidate) =>
            candidate.scope_kind === canvas.scope_kind && candidate.status !== "removed",
        );
        setLoaded({ key: scopeKey, canvases });
      })
      .catch(() => {
        if (requestRef.current === requestId)
          setLoaded({ key: scopeKey, canvases: EMPTY_CANVASES });
      });

    return () => {
      if (requestRef.current === requestId) requestRef.current += 1;
    };
  }, [canvas, isTaskCanvas, lifecycleRevision, scopeKey]);

  return loaded.key === scopeKey ? loaded.canvases : EMPTY_CANVASES;
}
