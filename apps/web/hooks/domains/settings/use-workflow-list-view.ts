"use client";

import { useCallback, useState } from "react";

export type WorkflowListView = "compact" | "detailed";

export const WORKFLOW_LIST_VIEW_STORAGE_KEY = "kandev:workflows:settingsListView:v1";

function readStoredView(): WorkflowListView {
  if (typeof window === "undefined") return "compact";
  try {
    return window.localStorage.getItem(WORKFLOW_LIST_VIEW_STORAGE_KEY) === "detailed"
      ? "detailed"
      : "compact";
  } catch {
    return "compact";
  }
}

/**
 * Per-browser Compact/Detailed preference for the Workflows settings list.
 * Missing, unknown, or unreadable values mean Compact. A failed write still
 * switches the view for this page visit; the choice just does not persist.
 * This is a presentation preference, never a settings draft.
 */
export function useWorkflowListView() {
  const [view, setViewState] = useState<WorkflowListView>(readStoredView);

  const setView = useCallback((next: WorkflowListView) => {
    setViewState(next);
    try {
      window.localStorage.setItem(WORKFLOW_LIST_VIEW_STORAGE_KEY, next);
    } catch {
      // Storage unavailable: the in-memory view is authoritative for this visit.
    }
  }, []);

  return { view, setView };
}
