"use client";

import { useCallback } from "react";
import { openContentSearchResult } from "@/lib/commands/content-search-selection";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";

export function useContentSearchResultOpener(
  setOpen: (open: boolean) => void,
  worktreePath: string | null,
  activeSessionId: string | null,
) {
  return useCallback(
    (result: WorkspaceContentSearchResult) => {
      if (!activeSessionId) return;
      setOpen(false);
      openContentSearchResult(result, worktreePath, activeSessionId);
    },
    [activeSessionId, setOpen, worktreePath],
  );
}
