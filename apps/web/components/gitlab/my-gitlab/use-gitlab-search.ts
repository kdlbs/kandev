"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { searchUserIssues, searchUserMRs } from "@/lib/api/domains/gitlab-api";
import type { Issue, MR } from "@/lib/types/gitlab";
import type { PresetOption } from "./presets";

type SearchKind = "mr" | "issue";
type Item = MR | Issue;

export const SEARCH_PAGE_SIZE = 25;

type SearchState = {
  items: Item[];
  loading: boolean;
  error: string | null;
  lastFetchedAt: Date | null;
  total: number;
};

type FetchArgs = {
  filter: string;
  customQuery: string;
  page: number;
};

type UseGitLabSearchOptions = {
  kind: SearchKind;
  presets: PresetOption[];
  preset: string;
  customQuery: string;
  projectFilter?: string;
  // Gate the network fetch on the integration being connected. When GitLab is
  // not configured the page renders the connect notice instead of the list, so
  // firing the search would only produce failing (500) requests in the console.
  enabled?: boolean;
};

export function pickFilter(
  presets: PresetOption[],
  preset: string,
  customQuery: string,
): { filter: string; customQuery: string } {
  const trimmed = customQuery.trim();
  if (trimmed) {
    return { filter: "", customQuery: trimmed };
  }
  const found = presets.find((p) => p.value === preset);
  return { filter: found?.filter ?? "", customQuery: "" };
}

// Apply project filter client-side. GitLab's global /merge_requests and
// /issues endpoints have no path_with_namespace qualifier — filtering server-
// side would require a project_id lookup. The dropdown is populated from the
// page's own results (useKnownProjects), so what's offered is always what's
// already shown — making this a UX narrowing more than a server query.
function filterByProject(items: Item[], project: string): Item[] {
  if (!project) return items;
  return items.filter((it) => it.project_path === project);
}

export function useGitLabSearch({
  kind,
  presets,
  preset,
  customQuery,
  projectFilter = "",
  enabled = true,
}: UseGitLabSearchOptions) {
  const [state, setState] = useState<SearchState>({
    items: [],
    loading: false,
    error: null,
    lastFetchedAt: null,
    total: 0,
  });
  const [page, setPage] = useState(1);
  const requestSeq = useRef(0);

  useEffect(() => {
    setPage(1);
  }, [preset, customQuery, kind]);

  const fetchData = useCallback(
    async ({ filter: ef, customQuery: ec, page: epage }: FetchArgs) => {
      if (!enabled) return;
      const seq = ++requestSeq.current;
      setState((s) => ({ ...s, loading: true, error: null }));
      try {
        const params = { filter: ef, customQuery: ec, page: epage, perPage: SEARCH_PAGE_SIZE };
        const response =
          kind === "mr" ? await searchUserMRs(params) : await searchUserIssues(params);
        if (seq !== requestSeq.current) return;
        const items: Item[] =
          kind === "mr"
            ? ((response as { mrs: MR[] | null } | null)?.mrs ?? [])
            : ((response as { issues: Issue[] | null } | null)?.issues ?? []);
        setState({
          items,
          loading: false,
          error: null,
          lastFetchedAt: new Date(),
          total: response?.total_count ?? items.length,
        });
      } catch (err) {
        if (seq !== requestSeq.current) return;
        setState((s) => ({
          items: [],
          loading: false,
          error: err instanceof Error ? err.message : "Failed to search GitLab",
          lastFetchedAt: s.lastFetchedAt,
          total: 0,
        }));
      }
    },
    [kind, enabled],
  );

  const resolved = useMemo(
    () => pickFilter(presets, preset, customQuery),
    [presets, preset, customQuery],
  );

  useEffect(() => {
    void fetchData({ filter: resolved.filter, customQuery: resolved.customQuery, page });
  }, [fetchData, resolved.filter, resolved.customQuery, page]);

  const refresh = useCallback(
    () => fetchData({ filter: resolved.filter, customQuery: resolved.customQuery, page }),
    [fetchData, resolved.filter, resolved.customQuery, page],
  );

  const filtered = useMemo(
    () => filterByProject(state.items, projectFilter),
    [state.items, projectFilter],
  );

  // `total` is the server-side count (used by pagination so the user can still
  // navigate to later pages that may contain more matches when projectFilter
  // is active). The consumer decides what to *display* as a count — see the
  // page client, which shows filtered.length next to the title when narrowing.
  return {
    items: filtered,
    rawItems: state.items,
    loading: state.loading,
    error: state.error,
    lastFetchedAt: state.lastFetchedAt,
    total: state.total,
    page,
    setPage,
    pageSize: SEARCH_PAGE_SIZE,
    refresh,
  };
}
