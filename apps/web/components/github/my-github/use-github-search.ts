"use client";

import { useCallback, useEffect, useState } from "react";
import { searchUserPRs, searchUserIssues } from "@/lib/api/domains/github-api";
import type { GitHubPR, GitHubIssue } from "@/lib/types/github";
import type { PresetOption } from "./search-bar";

type SearchKind = "pr" | "issue";

export const SEARCH_PAGE_SIZE = 25;

type SearchState<T> = {
  items: T[];
  loading: boolean;
  error: string | null;
  lastFetchedAt: Date | null;
  total: number;
};

type FetchArgs = {
  preset: string;
  customQuery: string;
  repoFilter: string;
  page: number;
};

function buildParams(
  presets: PresetOption[],
  preset: string,
  customQuery: string,
  repoFilter: string,
) {
  const trimmed = customQuery.trim();
  const repoQualifier = repoFilter ? `repo:${repoFilter}` : "";
  if (trimmed) {
    return { query: [trimmed, repoQualifier].filter(Boolean).join(" ") };
  }
  const found = presets.find((p) => p.value === preset);
  const filter = [found?.filter ?? "", repoQualifier].filter(Boolean).join(" ");
  return { filter };
}

export function useGitHubSearch<T extends GitHubPR | GitHubIssue>(
  kind: SearchKind,
  presets: PresetOption[],
  preset: string,
  customQuery: string,
  repoFilter: string = "",
) {
  const [state, setState] = useState<SearchState<T>>({
    items: [],
    loading: false,
    error: null,
    lastFetchedAt: null,
    total: 0,
  });
  const [page, setPage] = useState(1);

  // Reset to page 1 whenever the filter inputs change.
  useEffect(() => {
    setPage(1);
  }, [preset, customQuery, repoFilter, kind]);

  const fetchData = useCallback(
    async ({ preset: ep, customQuery: ec, repoFilter: er, page: epage }: FetchArgs) => {
      setState((s) => ({ ...s, loading: true, error: null }));
      try {
        const base = buildParams(presets, ep, ec, er);
        const params = { ...base, page: epage, perPage: SEARCH_PAGE_SIZE };
        const response =
          kind === "pr" ? await searchUserPRs(params) : await searchUserIssues(params);
        const items = (kind === "pr"
          ? (response as { prs: GitHubPR[] }).prs
          : (response as { issues: GitHubIssue[] }).issues) as unknown as T[];
        setState({
          items: items ?? [],
          loading: false,
          error: null,
          lastFetchedAt: new Date(),
          total: response.total_count ?? (items ?? []).length,
        });
      } catch (err) {
        setState((s) => ({
          items: [],
          loading: false,
          error: err instanceof Error ? err.message : "Failed to search GitHub",
          lastFetchedAt: s.lastFetchedAt,
          total: 0,
        }));
      }
    },
    [kind, presets],
  );

  useEffect(() => {
    void fetchData({ preset, customQuery, repoFilter, page });
  }, [fetchData, preset, customQuery, repoFilter, page]);

  const refresh = useCallback(
    () => fetchData({ preset, customQuery, repoFilter, page }),
    [fetchData, preset, customQuery, repoFilter, page],
  );

  return { ...state, page, setPage, pageSize: SEARCH_PAGE_SIZE, refresh };
}
