"use client";

import { useEffect, useRef, useState } from "react";
import { searchUserIssues, searchUserMRs } from "@/lib/api/domains/gitlab-api";
import type { Issue, MR } from "@/lib/types/gitlab";

type SearchState<T> = {
  items: T[];
  loading: boolean;
  error: string | null;
};

function initial<T>(): SearchState<T> {
  return { items: [], loading: true, error: null };
}

async function fetchMRs(
  filter: string,
  query: string,
  perPage: number,
  onResult: (s: SearchState<MR>) => void,
) {
  onResult({ items: [], loading: true, error: null });
  try {
    const res = await searchUserMRs({ filter, customQuery: query, perPage });
    onResult({ items: res?.mrs ?? [], loading: false, error: null });
  } catch (err) {
    onResult({
      items: [],
      loading: false,
      error: err instanceof Error ? err.message : "Failed to load MRs",
    });
  }
}

async function fetchIssues(
  filter: string,
  query: string,
  perPage: number,
  onResult: (s: SearchState<Issue>) => void,
) {
  onResult({ items: [], loading: true, error: null });
  try {
    const res = await searchUserIssues({ filter, customQuery: query, perPage });
    onResult({ items: res?.issues ?? [], loading: false, error: null });
  } catch (err) {
    onResult({
      items: [],
      loading: false,
      error: err instanceof Error ? err.message : "Failed to load issues",
    });
  }
}

/** Fetch the current user's MRs from GitLab. Re-runs whenever filter or query change. */
export function useGitLabUserMRs(filter: string, query: string, perPage = 50): SearchState<MR> {
  const [state, setState] = useState<SearchState<MR>>(initial);
  const requestRef = useRef(0);

  useEffect(() => {
    const requestId = ++requestRef.current;
    void fetchMRs(filter, query, perPage, (next) => {
      if (requestRef.current !== requestId) return;
      setState(next);
    });
  }, [filter, query, perPage]);

  return state;
}

/** Fetch the current user's issues from GitLab. */
export function useGitLabUserIssues(
  filter: string,
  query: string,
  perPage = 50,
): SearchState<Issue> {
  const [state, setState] = useState<SearchState<Issue>>(initial);
  const requestRef = useRef(0);

  useEffect(() => {
    const requestId = ++requestRef.current;
    void fetchIssues(filter, query, perPage, (next) => {
      if (requestRef.current !== requestId) return;
      setState(next);
    });
  }, [filter, query, perPage]);

  return state;
}
