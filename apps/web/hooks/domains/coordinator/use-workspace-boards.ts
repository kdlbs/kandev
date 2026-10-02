"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { listWorkflows } from "@/lib/api/domains/kanban-api";

export type WorkspaceBoard = { id: string; name: string; hidden: boolean };
export type BoardsStatus = "loading" | "ready" | "error";

/**
 * The workspace's boards in workspace order, hidden ones included. The latest
 * read wins; a failed re-read keeps the list already loaded.
 */
export function useWorkspaceBoards(workspaceId: string, enabled = true) {
  const [boards, setBoards] = useState<WorkspaceBoard[]>([]);
  const [status, setStatus] = useState<BoardsStatus>("loading");
  const sequenceRef = useRef(0);
  const loadedRef = useRef(false);

  const reload = useCallback(() => {
    const sequence = ++sequenceRef.current;
    listWorkflows(workspaceId, { includeHidden: true })
      .then((response) => {
        if (sequence !== sequenceRef.current) return;
        const ordered = [...response.workflows].sort(
          (a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0),
        );
        loadedRef.current = true;
        setBoards(ordered.map((w) => ({ id: w.id, name: w.name, hidden: Boolean(w.hidden) })));
        setStatus("ready");
      })
      .catch(() => {
        if (sequence !== sequenceRef.current || loadedRef.current) return;
        setStatus("error");
      });
  }, [workspaceId]);

  useEffect(() => {
    if (!enabled) return;
    loadedRef.current = false;
    setBoards([]);
    setStatus("loading");
    reload();
    return () => {
      sequenceRef.current += 1;
    };
  }, [enabled, reload]);

  const retry = useCallback(() => {
    setStatus("loading");
    reload();
  }, [reload]);

  return { boards, status, retry };
}
