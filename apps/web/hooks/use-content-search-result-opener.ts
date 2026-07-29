"use client";

import { useCallback } from "react";
import { openContentSearchResult } from "@/lib/commands/content-search-selection";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";

export function useContentSearchResultOpener(
  setOpen: (open: boolean) => void,
  worktreePath: string | null,
) {
  return useCallback(
    (result: WorkspaceContentSearchResult) => {
      setOpen(false);
      openContentSearchResult(result, worktreePath);
    },
    [setOpen, worktreePath],
  );
}
