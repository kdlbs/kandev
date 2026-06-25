"use client";

import { useCallback, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { officeRoutingPreviewQueryOptions } from "@/lib/query/query-options/office";
import type { AgentRoutePreview } from "@/lib/state/slices/office/types";
import { queryErrorMessage } from "./query-error";

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
  const query = useQuery(officeRoutingPreviewQueryOptions(workspaceName ?? ""));

  const refresh = useCallback(async () => {
    if (!workspaceName) return;
    await query.refetch();
  }, [query, workspaceName]);

  useEffect(() => {
    if (!workspaceName || !query.data) return;
    setRoutingPreview(workspaceName, query.data.agents ?? []);
  }, [query.data, setRoutingPreview, workspaceName]);

  const queryAgents = query.data?.agents ?? agents;
  const error = queryErrorMessage(query.error);

  return { agents: queryAgents, isLoading: query.isPending, error, refresh };
}
