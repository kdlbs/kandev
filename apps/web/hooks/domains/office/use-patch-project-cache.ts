"use client";

import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import type { Project } from "@/lib/state/slices/office/types";

/**
 * Returns a function that optimistically patches a project in the office
 * projects TQ cache for the active workspace.
 *
 * Replaces the legacy `useAppStore(s => s.updateProject)` mirror write:
 * project detail edits patch the cached list so the projects picker /
 * sidebar / list reflect the change immediately, before the server
 * round-trip and any WS-driven invalidation reconcile it.
 */
export function usePatchProjectCache(): (projectId: string, patch: Partial<Project>) => void {
  const qc = useQueryClient();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);

  return useCallback(
    (projectId: string, patch: Partial<Project>) => {
      if (!workspaceId) return;
      qc.setQueryData<Project[]>(["office", workspaceId, "projects"], (prev) =>
        prev?.map((p) => (p.id === projectId ? { ...p, ...patch } : p)),
      );
    },
    [qc, workspaceId],
  );
}
