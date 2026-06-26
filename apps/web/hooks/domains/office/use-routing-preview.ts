"use client";

import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
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
  const query = useQuery(officeRoutingPreviewQueryOptions(workspaceName ?? ""));

  const refresh = useCallback(async () => {
    if (!workspaceName) return;
    await query.refetch();
  }, [query, workspaceName]);

  const queryAgents = query.data?.agents ?? EMPTY_PREVIEW;
  const error = queryErrorMessage(query.error);

  return { agents: queryAgents, isLoading: query.isPending, error, refresh };
}
