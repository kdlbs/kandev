"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getRoutingPreview } from "@/lib/api/domains/office-extended-api";
import type { AgentRoutePreview } from "@/lib/state/slices/office/types";

export type UseRoutingPreviewResult = {
  agents: AgentRoutePreview[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

const EMPTY_PREVIEW: AgentRoutePreview[] = [];

export function useRoutingPreview(workspaceName: string | null): UseRoutingPreviewResult {
  const agents = useAppStore((s) =>
    workspaceName
      ? (s.office.routing.preview.byWorkspace[workspaceName] ?? EMPTY_PREVIEW)
      : EMPTY_PREVIEW,
  );
  const setRoutingPreview = useAppStore((s) => s.setRoutingPreview);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fetched, setFetched] = useState(false);

  const refresh = useCallback(async () => {
    if (!workspaceName) return;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getRoutingPreview(workspaceName);
      setRoutingPreview(workspaceName, res.agents ?? []);
      setFetched(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load routing preview");
    } finally {
      setIsLoading(false);
    }
  }, [workspaceName, setRoutingPreview]);

  useEffect(() => {
    if (!workspaceName || fetched) return;
    void refresh();
  }, [workspaceName, fetched, refresh]);

  return { agents, isLoading, error, refresh };
}
