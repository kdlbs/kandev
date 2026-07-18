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
  const requestId = useRef(0);
  const search = useCallback(
    async (request: { project: string; wiql: string; top?: number }) => {
      if (!workspaceId) return;
      const current = ++requestId.current;
      setState((previous) => ({ ...previous, loading: true, error: null }));
      try {
        const result = await searchAzureDevOpsWorkItems(workspaceId, request, {
          cache: "no-store",
        });
        if (current === requestId.current) {
          setState({ data: result.items ?? [], loading: false, error: null });
        }
      } catch (err) {
        if (current === requestId.current) {
          setState((previous) => ({ ...previous, loading: false, error: String(err) }));
        }
      }
    },
    [workspaceId],
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
  const requestId = useRef(0);
  const search = useCallback(
    async (filters: AzureDevOpsPullRequestFilters) => {
      if (!workspaceId) return;
      const current = ++requestId.current;
      setState((previous) => ({ ...previous, loading: true, error: null }));
      try {
        const result = await listAzureDevOpsPullRequests(workspaceId, filters, {
          cache: "no-store",
        });
        if (current === requestId.current) {
          setState({
            data: result.items ?? [],
            count: result.count ?? 0,
            loading: false,
            error: null,
          });
        }
      } catch (err) {
        if (current === requestId.current) {
          setState((previous) => ({ ...previous, loading: false, error: String(err) }));
        }
      }
    },
    [workspaceId],
  );
  return { ...state, search };
}

export function useAzureDevOpsPullRequestFeedback(workspaceId?: string) {
  const [state, setState] = useState<AsyncResult<AzureDevOpsPullRequestFeedback | null>>({
    data: null,
    loading: false,
    error: null,
  });
  const load = useCallback(
    async (pullRequest: AzureDevOpsPullRequest) => {
      if (!workspaceId) return;
      setState({ data: null, loading: true, error: null });
      try {
        const data = await getAzureDevOpsPullRequestFeedback(
          workspaceId,
          pullRequest.projectId,
          pullRequest.repositoryId,
          pullRequest.id,
          { cache: "no-store" },
        );
        setState({ data, loading: false, error: null });
      } catch (err) {
        setState({ data: null, loading: false, error: String(err) });
      }
    },
    [workspaceId],
  );
  const clear = useCallback(() => {
    setState({ data: null, loading: false, error: null });
  }, []);
  return { ...state, load, clear };
}
