"use client";

import { useEffect, useState } from "react";
import { searchTasks } from "@/lib/api/domains/office-extended-api";
import type { OfficeTask } from "@/lib/state/slices/office/types";

// Loads a bounded, workspace-scoped candidate list for the automation parent
// picker. Reuses the shared picker search endpoint (same one the parent and
// blockers pickers use) so an unknown or foreign task can never be offered.
export function useAutomationParentCandidates(workspaceId: string, query = "") {
  const [result, setResult] = useState<{ workspaceId: string; query: string; tasks: OfficeTask[] }>(
    {
      workspaceId: "",
      query: "",
      tasks: [],
    },
  );
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    setLoading(true);
    searchTasks(workspaceId, query, 50)
      .then((response) => {
        if (!cancelled) setResult({ workspaceId, query, tasks: response.tasks ?? [] });
      })
      .catch(() => {
        if (!cancelled) setResult({ workspaceId, query, tasks: [] });
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, query]);

  return {
    candidates: result.workspaceId === workspaceId && result.query === query ? result.tasks : [],
    loading,
  };
}
