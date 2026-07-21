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

type AsyncResult<T> = {
  data: T;
  loading: boolean;
  error: string | null;
};

function useOperationGeneration(scope?: string) {
  const generation = useRef({ scope, value: 0 });
  if (generation.current.scope !== scope) {
    generation.current = { scope, value: generation.current.value + 1 };
  }
  return generation;
}

export function useAzureDevOpsConnection(workspaceId?: string) {
  const [state, setState] = useState<AsyncResult<AzureDevOpsConfig | null>>({
    data: null,
    loading: true,
    error: null,
  });
  useEffect(() => {
    if (!workspaceId) {
      setState({ data: null, loading: false, error: null });
      return;
    }
    let cancelled = false;
    setState({ data: null, loading: true, error: null });
    getAzureDevOpsConfig(workspaceId, { cache: "no-store" })
      .then((data) => {
        if (!cancelled) setState({ data, loading: false, error: null });
      })
      .catch((err) => {
        if (!cancelled) setState({ data: null, loading: false, error: String(err) });
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);
  return state;
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
