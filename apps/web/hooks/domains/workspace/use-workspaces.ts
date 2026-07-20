"use client";

import { useEffect, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { workspacesQueryOptions } from "@/lib/query/query-options";
import type { Workspace } from "@/lib/types/http";

const EMPTY_WORKSPACES: Workspace[] = [];

export function useWorkspaces() {
  const query = useQuery(workspacesQueryOptions());
  const items = query.data ?? EMPTY_WORKSPACES;
  const activeId = useAppStore((state) => state.workspaces.activeId);
  const setActiveWorkspace = useAppStore((state) => state.setActiveWorkspace);
  const setActiveWorkflow = useAppStore((state) => state.setActiveWorkflow);

  useEffect(() => {
    if (query.data === undefined) return;
    const nextActiveId = resolveActiveWorkspaceId(items, activeId);
    if (nextActiveId === activeId) return;
    setActiveWorkspace(nextActiveId);
    if (activeId && nextActiveId !== activeId) setActiveWorkflow(null);
  }, [activeId, items, query.data, setActiveWorkflow, setActiveWorkspace]);

  const activeWorkspace = useMemo(
    () => items.find((workspace) => workspace.id === activeId) ?? null,
    [activeId, items],
  );

  return {
    items,
    activeId,
    activeWorkspace,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    query,
  };
}

function resolveActiveWorkspaceId(items: Workspace[], activeId: string | null): string | null {
  if (items.length === 0) return null;
  if (activeId && items.some((workspace) => workspace.id === activeId)) return activeId;
  return items[0].id;
}
