"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  getAzureDevOpsConfig,
  getAzureDevOpsPullRequestFeedback,
  listAzureDevOpsPullRequests,
  searchAzureDevOpsWorkItems,
  type AzureDevOpsPullRequestFilters,
} from "@/lib/api/domains/azure-devops-api";
import type {
  AzureDevOpsConfig,
  AzureDevOpsPullRequest,
  AzureDevOpsPullRequestFeedback,
  AzureDevOpsWorkItem,
} from "@/lib/types/azure-devops";
import { subscribeIntegrationAvailability } from "@/lib/integrations/integration-availability-events";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";

type AsyncResult<T> = {
  data: T;
  loading: boolean;
  error: string | null;
};

type AzureDevOpsConnectionState = AsyncResult<AzureDevOpsConfig | null> & {
  workspaceId?: string;
  refreshing: boolean;
};

function useOperationGeneration(scope?: string) {
  const generation = useRef({ scope, value: 0 });
  if (generation.current.scope !== scope) {
    generation.current = { scope, value: generation.current.value + 1 };
  }
  return generation;
}

export function useAzureDevOpsConnection(workspaceId?: string) {
  const [refreshVersion, setRefreshVersion] = useState(0);
  const [state, setState] = useState<AzureDevOpsConnectionState>({
    workspaceId,
    data: null,
    loading: true,
    error: null,
    refreshing: false,
  });
  useEffect(() => {
    let cancelled = false;
    let requestId = 0;
    const load = async () => {
      const currentRequestId = ++requestId;
      setState((previous) => {
        const sameWorkspace = previous.workspaceId === workspaceId;
        const backgroundRefresh = sameWorkspace && !previous.loading;
        return {
          workspaceId,
          data: sameWorkspace ? previous.data : null,
          loading: Boolean(workspaceId) && !backgroundRefresh,
          error: null,
          refreshing: Boolean(workspaceId) && backgroundRefresh,
        };
      });
      if (!workspaceId) return;
      try {
        const data = await getAzureDevOpsConfig(workspaceId, { cache: "no-store" });
        if (!cancelled && currentRequestId === requestId) {
          setState({ workspaceId, data, loading: false, error: null, refreshing: false });
        }
      } catch (err) {
        if (!cancelled && currentRequestId === requestId) {
          setState((previous) => ({
            workspaceId,
            data: previous.workspaceId === workspaceId ? previous.data : null,
            loading: false,
            error: String(err),
            refreshing: false,
          }));
        }
      }
    };
    void load();
    if (!workspaceId) {
      return () => {
        cancelled = true;
      };
    }
    const interval = setInterval(() => void load(), INTEGRATION_STATUS_REFRESH_MS);
    const unsubscribe = subscribeIntegrationAvailability(() => void load());
    return () => {
      cancelled = true;
      clearInterval(interval);
      unsubscribe();
    };
  }, [refreshVersion, workspaceId]);
  const refresh = useCallback(() => setRefreshVersion((version) => version + 1), []);
  const scopedState =
    state.workspaceId === workspaceId
      ? state
      : { workspaceId, data: null, loading: Boolean(workspaceId), error: null, refreshing: false };
  return { ...scopedState, refresh };
}

export function useAzureDevOpsWorkItemSearch(workspaceId?: string) {
  const [state, setState] = useState<AsyncResult<AzureDevOpsWorkItem[]>>({
    data: [],
    loading: false,
    error: null,
  });
  const generation = useOperationGeneration(workspaceId);
  useEffect(() => {
    setState({ data: [], loading: false, error: null });
  }, [workspaceId]);
  const search = useCallback(
    async (request: { project: string; wiql: string; top?: number }) => {
      if (!workspaceId) return;
      const current = ++generation.current.value;
      setState((previous) => ({ ...previous, loading: true, error: null }));
      try {
        const result = await searchAzureDevOpsWorkItems(workspaceId, request, {
          cache: "no-store",
        });
        if (current === generation.current.value) {
          setState({ data: result.items ?? [], loading: false, error: null });
        }
      } catch (err) {
        if (current === generation.current.value) {
          setState((previous) => ({ ...previous, loading: false, error: String(err) }));
        }
      }
    },
    [generation, workspaceId],
  );
  return { ...state, search };
}

export function useAzureDevOpsPullRequestSearch(workspaceId?: string) {
  const [state, setState] = useState<AsyncResult<AzureDevOpsPullRequest[]> & { count: number }>({
    data: [],
    count: 0,
    loading: false,
    error: null,
  });
  const generation = useOperationGeneration(workspaceId);
  useEffect(() => {
    setState({ data: [], count: 0, loading: false, error: null });
  }, [workspaceId]);
  const search = useCallback(
    async (filters: AzureDevOpsPullRequestFilters) => {
      if (!workspaceId) return;
      const current = ++generation.current.value;
      setState((previous) => ({ ...previous, loading: true, error: null }));
      try {
        const result = await listAzureDevOpsPullRequests(workspaceId, filters, {
          cache: "no-store",
        });
        if (current === generation.current.value) {
          setState({
            data: result.items ?? [],
            count: result.count ?? 0,
            loading: false,
            error: null,
          });
        }
      } catch (err) {
        if (current === generation.current.value) {
          setState((previous) => ({ ...previous, loading: false, error: String(err) }));
        }
      }
    },
    [generation, workspaceId],
  );
  return { ...state, search };
}

export function useAzureDevOpsPullRequestFeedback(workspaceId?: string) {
  const [state, setState] = useState<AsyncResult<AzureDevOpsPullRequestFeedback | null>>({
    data: null,
    loading: false,
    error: null,
  });
  const generation = useOperationGeneration(workspaceId);
  useEffect(() => {
    setState({ data: null, loading: false, error: null });
  }, [workspaceId]);
  const load = useCallback(
    async (pullRequest: AzureDevOpsPullRequest) => {
      if (!workspaceId) return;
      const current = ++generation.current.value;
      setState({ data: null, loading: true, error: null });
      try {
        const data = await getAzureDevOpsPullRequestFeedback(
          workspaceId,
          pullRequest.projectId,
          pullRequest.repositoryId,
          pullRequest.id,
          { cache: "no-store" },
        );
        if (current === generation.current.value) {
          setState({ data, loading: false, error: null });
        }
      } catch (err) {
        if (current === generation.current.value) {
          setState({ data: null, loading: false, error: String(err) });
        }
      }
    },
    [generation, workspaceId],
  );
  const clear = useCallback(() => {
    generation.current.value += 1;
    setState({ data: null, loading: false, error: null });
  }, [generation]);
  return { ...state, load, clear };
}
