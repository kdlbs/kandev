"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { searchUserIssues, searchUserMRs } from "@/lib/api/domains/gitlab-api";
import type { Issue, MR } from "@/lib/types/gitlab";
import type { PresetOption } from "./presets";
import { t } from "@/lib/i18n";

type SearchKind = "mr" | "issue";
type Item = MR | Issue;

export const SEARCH_PAGE_SIZE = 25;

type SearchState = {
  items: Item[];
  loading: boolean;
  error: string | null;
  lastFetchedAt: Date | null;
  total: number;
  workspaceId: string;
};

type FetchArgs = {
  filter: string;
  customQuery: string;
  milestone: string;
  page: number;
};

type UseGitLabSearchOptions = {
  workspaceId: string;
  kind: SearchKind;
  presets: PresetOption[];
  preset: string;
  customQuery: string;
  milestone?: string;
  projectFilter?: string;
  // Gate the network fetch on the integration being connected. When GitLab is
  // not configured the page renders the connect notice instead of the list, so
  // firing the search would only produce failing (500) requests in the console.
  enabled?: boolean;
};

function initialSearchState(workspaceId: string): SearchState {
  return {
    items: [],
    loading: false,
    error: null,
    lastFetchedAt: null,
    total: 0,
    workspaceId,
  };
}

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

async function fetchGitLabSearchPage(
  kind: SearchKind,
  workspaceId: string,
  args: FetchArgs,
): Promise<Omit<SearchState, "workspaceId">> {
  const params = {
    workspaceId,
    filter: args.filter,
    customQuery: args.customQuery,
    page: args.page,
    perPage: SEARCH_PAGE_SIZE,
  };
  const response =
    kind === "mr"
      ? await searchUserMRs(params)
      : await searchUserIssues({ ...params, milestone: args.milestone });
  const items: Item[] =
    kind === "mr"
      ? ((response as { mrs: MR[] | null } | null)?.mrs ?? [])
      : ((response as { issues: Issue[] | null } | null)?.issues ?? []);
  return {
    items,
    loading: false,
    error: null,
    lastFetchedAt: new Date(),
    total: response?.total_count ?? items.length,
  };
}

export function useGitLabSearch({
  workspaceId,
  kind,
  presets,
  preset,
  customQuery,
  milestone = "",
  projectFilter = "",
  enabled = true,
}: UseGitLabSearchOptions) {
  const [state, setState] = useState<SearchState>(() => initialSearchState(workspaceId));
  const [page, setPage] = useState(1);
  const requestSeq = useRef(0);

  useEffect(() => {
    setPage(1);
  }, [preset, customQuery, kind]);

  const fetchData = useCallback(
    async (args: FetchArgs) => {
      if (!enabled || !workspaceId) return;
      const seq = ++requestSeq.current;
      const requestedWorkspaceId = workspaceId;
      setState((s) =>
        s.workspaceId === requestedWorkspaceId
          ? { ...s, loading: true, error: null }
          : {
              items: [],
              loading: true,
              error: null,
              lastFetchedAt: null,
              total: 0,
              workspaceId: requestedWorkspaceId,
            },
      );
      try {
        const next = await fetchGitLabSearchPage(kind, workspaceId, args);
        if (seq !== requestSeq.current) return;
        setState({ ...next, workspaceId: requestedWorkspaceId });
      } catch (err) {
        if (seq !== requestSeq.current) return;
        setState((s) => ({
          items: [],
          loading: false,
          error: err instanceof Error ? err.message : t("gitlab:failedToSearchGitLab"),
          lastFetchedAt: s.lastFetchedAt,
          total: 0,
          workspaceId: requestedWorkspaceId,
        }));
      }
    },
    [kind, enabled, workspaceId],
  );

  const resolved = useMemo(
    () => pickFilter(presets, preset, customQuery),
    [presets, preset, customQuery],
  );

  useEffect(() => {
    // Gate at the effect level too (mirrors the Linear hook, which guards both
    // its run callback and its debounce effect) so a disabled integration never
    // schedules a fetch. The callback keeps its own guard to also cover refresh().
    if (!enabled || !workspaceId) return;
    void fetchData({
      filter: resolved.filter,
      customQuery: resolved.customQuery,
      milestone,
      page,
    });
  }, [fetchData, enabled, workspaceId, resolved.filter, resolved.customQuery, milestone, page]);

  const refresh = useCallback(
    () =>
      fetchData({ filter: resolved.filter, customQuery: resolved.customQuery, milestone, page }),
    [fetchData, resolved.filter, resolved.customQuery, milestone, page],
  );

  const isCurrentWorkspace = state.workspaceId === workspaceId;
  const currentItems = enabled && workspaceId && isCurrentWorkspace ? state.items : [];
  const filtered = useMemo(
    () => filterByProject(currentItems, projectFilter),
    [currentItems, projectFilter],
  );

  // `total` is the server-side count (used by pagination so the user can still
  // navigate to later pages that may contain more matches when projectFilter
  // is active). The consumer decides what to *display* as a count — see the
  // page client, which shows filtered.length next to the title when narrowing.
  return {
    items: filtered,
    rawItems: currentItems,
    loading: enabled && Boolean(workspaceId) && (!isCurrentWorkspace || state.loading),
    error: isCurrentWorkspace ? state.error : null,
    lastFetchedAt: state.lastFetchedAt,
    total: isCurrentWorkspace ? state.total : 0,
    page,
    setPage,
    pageSize: SEARCH_PAGE_SIZE,
    refresh,
  };
}
