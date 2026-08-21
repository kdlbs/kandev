"use client";

import { useAppStore } from "@/components/state-provider";
import type { Repository } from "@/lib/types/http";

const EMPTY_REPOSITORIES: Repository[] = [];

export function useActiveWorkspaceRepositories() {
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  return useAppStore((state) =>
    activeWorkspaceId
      ? (state.repositories.itemsByWorkspaceId[activeWorkspaceId] ?? EMPTY_REPOSITORIES)
      : EMPTY_REPOSITORIES,
  );
}
