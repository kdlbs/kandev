"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { normalizeLocale } from "@/lib/i18n";
import { sidebarSortToWire } from "@/lib/sidebar/sidebar-sort-chain";
import {
  composeMobileListQuery,
  resolveMobileListLinkScope,
  type MobileListLinkScope,
} from "@/lib/tasks/mobile-list-query";
import type { FilterClause, SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import type { SidebarTaskQuery } from "@/lib/types/http";
import type { Repository } from "@/lib/types/http";
import { repositorySlug } from "@/lib/repository-slug";
import { useTranslation } from "react-i18next";

type ScopedResultsState = {
  identity: string;
  searchQuery: string;
  resultFilters: FilterClause[];
  filterDraft: FilterClause[];
  facetSortOverride: string | null | undefined;
  facetGroupOverride: string | null | undefined;
};

function createResultsState(identity: string, linkQuery?: string): ScopedResultsState {
  return {
    identity,
    searchQuery: linkQuery ?? "",
    resultFilters: [],
    filterDraft: [],
    facetSortOverride: undefined,
    facetGroupOverride: undefined,
  };
}

function routeScopeParameters(params: URLSearchParams): string[] {
  return ["archive", "query", "q", "workflow", "repository", "sort", "group"].filter((key) =>
    params.has(key),
  );
}

type MobileListQueryScopeOptions = {
  view: SidebarView;
  locale: string;
  state: ScopedResultsState;
  linkScope: MobileListLinkScope | null;
};

function useMobileListQueryScope(options: MobileListQueryScopeOptions) {
  const { view, locale, state, linkScope } = options;
  const savedQuery = useMemo<SidebarTaskQuery>(
    () => ({
      filters: view.filters.map(({ dimension, op, value }) => ({ dimension, op, value })),
      sort: sidebarSortToWire(view.sort),
      group: view.group,
      collapsed_group_keys: view.collapsedGroups,
      collapsed_task_ids: [],
      page: 1,
      page_size: 100,
      locale,
    }),
    [locale, view],
  );
  const composedQuery = useMemo(
    () =>
      composeMobileListQuery(savedQuery, {
        searchQuery: state.searchQuery,
        resultFilters: state.resultFilters,
        linkScope: linkScope ?? undefined,
      }),
    [linkScope, savedQuery, state.resultFilters, state.searchQuery],
  );
  const hasLinkScope = linkScope !== null;
  const hasOverrides = Boolean(hasLinkScope || state.searchQuery || state.resultFilters.length > 0);
  const queryOverrides = useMemo<
    Pick<SidebarTaskQuery, "filters" | "sort" | "group" | "query"> | undefined
  >(
    () =>
      hasOverrides
        ? {
            filters: composedQuery.filters,
            sort: composedQuery.sort,
            group: composedQuery.group,
            ...(composedQuery.query ? { query: composedQuery.query } : {}),
          }
        : undefined,
    [composedQuery, hasOverrides],
  );
  return { hasLinkScope, queryOverrides };
}

type ResultStateUpdater = (update: (current: ScopedResultsState) => ScopedResultsState) => void;

type MobileListResultActionsOptions = {
  router: ReturnType<typeof useRouter>;
  serializedParams: string;
  setDismissedRouteSignature: (signature: string | null) => void;
  hasLinkScope: boolean;
  workspaceId: string | null;
  viewId: string;
  updateState: ResultStateUpdater;
  setStoredState: Dispatch<SetStateAction<ScopedResultsState>>;
};

function useFacetResultActions(updateState: ResultStateUpdater) {
  const setFacetSortOverride = useCallback(
    (facetKey: string | null) =>
      updateState((current) => ({ ...current, facetSortOverride: facetKey })),
    [updateState],
  );
  const setFacetGroupOverride = useCallback(
    (facetKey: string | null) =>
      updateState((current) => ({ ...current, facetGroupOverride: facetKey })),
    [updateState],
  );
  const resetFacetOverrides = useCallback(
    () =>
      updateState((current) => ({ ...current, facetSortOverride: null, facetGroupOverride: null })),
    [updateState],
  );
  return { setFacetSortOverride, setFacetGroupOverride, resetFacetOverrides };
}

function useMobileListResultActions(options: MobileListResultActionsOptions) {
  const {
    router,
    serializedParams,
    setDismissedRouteSignature,
    hasLinkScope,
    workspaceId,
    viewId,
    updateState,
    setStoredState,
  } = options;
  const facetActions = useFacetResultActions(updateState);
  const clearRouteScope = useCallback(() => {
    const params = new URLSearchParams(serializedParams);
    for (const key of routeScopeParameters(params)) params.delete(key);
    setDismissedRouteSignature(serializedParams);
    const query = params.toString();
    router.replace(`${window.location.pathname}${query ? `?${query}` : ""}`, { scroll: false });
  }, [router, serializedParams, setDismissedRouteSignature]);
  const setSearchQuery = useCallback(
    (searchQuery: string) => updateState((current) => ({ ...current, searchQuery })),
    [updateState],
  );
  const clearSearchAndFilters = useCallback(() => {
    updateState((current) => ({
      ...current,
      searchQuery: "",
      resultFilters: [],
      filterDraft: [],
      facetSortOverride: null,
      facetGroupOverride: null,
    }));
    if (hasLinkScope) clearRouteScope();
  }, [clearRouteScope, hasLinkScope, updateState]);
  const clearAllResults = useCallback(() => clearSearchAndFilters(), [clearSearchAndFilters]);
  const selectView = useCallback(
    (nextViewId: string) => {
      clearRouteScope();
      setStoredState(createResultsState(`${workspaceId ?? ""}:${nextViewId}:${serializedParams}`));
    },
    [clearRouteScope, serializedParams, setStoredState, workspaceId],
  );
  const selectWorkspace = useCallback(() => {
    clearRouteScope();
    setStoredState(createResultsState(`${workspaceId ?? ""}:${viewId}:${serializedParams}`));
  }, [clearRouteScope, serializedParams, setStoredState, viewId, workspaceId]);
  const beginFilterEdit = useCallback(
    () => updateState((current) => ({ ...current, filterDraft: [...current.resultFilters] })),
    [updateState],
  );
  const setFilterDraft = useCallback(
    (filterDraft: FilterClause[]) => updateState((current) => ({ ...current, filterDraft })),
    [updateState],
  );
  const cancelFilterEdit = useCallback(
    () => updateState((current) => ({ ...current, filterDraft: [...current.resultFilters] })),
    [updateState],
  );
  const applyFilterEdit = useCallback(
    () => updateState((current) => ({ ...current, resultFilters: [...current.filterDraft] })),
    [updateState],
  );
  const clearFilterEdit = useCallback(
    () => updateState((current) => ({ ...current, resultFilters: [], filterDraft: [] })),
    [updateState],
  );
  const clearFilterDraft = useCallback(
    () => updateState((current) => ({ ...current, filterDraft: [] })),
    [updateState],
  );
  const removeResultFilter = useCallback(
    (dimension: FilterClause["dimension"]) =>
      updateState((current) => ({
        ...current,
        resultFilters: current.resultFilters.filter((filter) => filter.dimension !== dimension),
        filterDraft: current.filterDraft.filter((filter) => filter.dimension !== dimension),
      })),
    [updateState],
  );
  return {
    ...facetActions,
    setSearchQuery,
    clearSearchAndFilters,
    clearAllResults,
    selectView,
    selectWorkspace,
    beginFilterEdit,
    setFilterDraft,
    cancelFilterEdit,
    applyFilterEdit,
    clearFilterEdit,
    clearFilterDraft,
    removeResultFilter,
  };
}

export function useMobileListResults(
  workspaceId: string | null,
  view: SidebarView,
  repositories: Repository[] = [],
) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { i18n } = useTranslation();
  const serializedParams = searchParams.toString();
  const repositorySlugById = useMemo(
    () => new Map(repositories.map((repository) => [repository.id, repositorySlug(repository)])),
    [repositories],
  );
  const routeScope = useMemo(
    () => resolveMobileListLinkScope(new URLSearchParams(serializedParams), repositorySlugById),
    [repositorySlugById, serializedParams],
  );
  const [dismissedRouteSignature, setDismissedRouteSignature] = useState<string | null>(null);
  useEffect(() => {
    if (dismissedRouteSignature && serializedParams !== dismissedRouteSignature) {
      setDismissedRouteSignature(null);
    }
  }, [dismissedRouteSignature, serializedParams]);
  const linkScope = dismissedRouteSignature === serializedParams ? null : routeScope;
  const identity = `${workspaceId ?? ""}:${view.id}:${serializedParams}`;
  const emptyState = useMemo(
    () => createResultsState(identity, linkScope?.query),
    [identity, linkScope?.query],
  );
  const [storedState, setStoredState] = useState(emptyState);
  const state = storedState.identity === identity ? storedState : emptyState;

  const updateState = useCallback(
    (update: (current: ScopedResultsState) => ScopedResultsState) => {
      setStoredState((current) => update(current.identity === identity ? current : emptyState));
    },
    [emptyState, identity],
  );

  const { hasLinkScope, queryOverrides } = useMobileListQueryScope({
    view,
    locale: normalizeLocale(i18n.resolvedLanguage ?? i18n.language),
    state,
    linkScope,
  });

  const actions = useMobileListResultActions({
    router,
    serializedParams,
    setDismissedRouteSignature,
    hasLinkScope,
    workspaceId,
    viewId: view.id,
    updateState,
    setStoredState,
  });

  return {
    searchQuery: state.searchQuery,
    ...actions,
    resultFilters: state.resultFilters,
    filterDraft: state.filterDraft,
    queryOverrides,
    hasLinkScope,
    linkScope,
    hasResultFilters: state.resultFilters.length > 0,
    activeFilterCount: state.resultFilters.length,
    facetSortOverride:
      state.facetSortOverride === undefined
        ? (linkScope?.facetSort ?? null)
        : state.facetSortOverride,
    facetGroupOverride:
      state.facetGroupOverride === undefined
        ? (linkScope?.facetGroup ?? null)
        : state.facetGroupOverride,
  };
}
