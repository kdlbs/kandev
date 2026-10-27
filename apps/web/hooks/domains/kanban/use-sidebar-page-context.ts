import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import { useAppStore } from "@/components/state-provider";
import {
  sidebarTaskPageRankingKey,
  sidebarTaskPageScope,
} from "@/lib/sidebar/sidebar-task-page-cache";
import { normalizeLocale } from "@/lib/i18n";
import type { SidebarTaskQuery } from "@/lib/types/http";
import type { FilterClause, SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { sidebarSortFromWire, sidebarSortToWire } from "@/lib/sidebar/sidebar-sort-chain";

const PAGE_SIZE = 100;
export type SidebarTaskQueryOverrides = Partial<
  Pick<SidebarTaskQuery, "filters" | "sort" | "group" | "query">
>;

export function resolveSidebarTaskView(
  view: SidebarView,
  overrides?: SidebarTaskQueryOverrides,
): SidebarView {
  if (!overrides) return view;
  return {
    ...view,
    filters:
      overrides.filters === undefined
        ? view.filters
        : overrides.filters.map((filter, index) => ({
            id: `query-${index}`,
            dimension: filter.dimension as FilterClause["dimension"],
            op: filter.op as FilterClause["op"],
            value: filter.value as FilterClause["value"],
          })),
    sort: overrides.sort === undefined ? view.sort : sidebarSortFromWire(overrides.sort),
    group: overrides.group === undefined ? view.group : (overrides.group as SidebarView["group"]),
    collapsedGroups: overrides.group === undefined ? view.collapsedGroups : [],
  };
}

function buildSidebarQuery(
  view: ReturnType<typeof useEffectiveSidebarView>,
  page: number,
  locale: string,
  collapsedTaskIDs: string[],
  textQuery?: string,
): SidebarTaskQuery {
  const query: SidebarTaskQuery = {
    filters: view.filters.map(({ dimension, op, value }) => ({ dimension, op, value })),
    sort: sidebarSortToWire(view.sort),
    group: view.group,
    collapsed_group_keys: view.collapsedGroups,
    collapsed_task_ids: collapsedTaskIDs,
    page,
    page_size: PAGE_SIZE,
    locale,
  };
  if (textQuery) query.query = textQuery;
  return query;
}

function useSidebarViewKey(
  workspaceId: string | null,
  workspaceGeneration: number,
  contextScope: string,
  queryView: SidebarTaskQuery,
  prefs: Pick<
    ReturnType<typeof useSidebarTaskPrefs>,
    "pinnedTaskIds" | "orderedTaskIds" | "subtaskOrderByParentId"
  >,
) {
  const { pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId } = prefs;
  return useMemo(
    () =>
      JSON.stringify({
        workspaceId,
        workspaceGeneration,
        contextScope,
        ...queryView,
        pinnedTaskIds,
        orderedTaskIds,
        subtaskOrderByParentId,
      }),
    [
      orderedTaskIds,
      pinnedTaskIds,
      queryView,
      subtaskOrderByParentId,
      workspaceId,
      workspaceGeneration,
      contextScope,
    ],
  );
}

/** Display continuity excludes only disclosure from the full query and context identity. */
export function sidebarContentKey(viewKey: string): string {
  if (!viewKey) return "";
  const identity: Record<string, unknown> = JSON.parse(viewKey);
  delete identity.collapsed_group_keys;
  delete identity.collapsed_task_ids;
  return JSON.stringify(identity);
}

export function useSidebarPageContext(
  workspaceId: string | null,
  queryOverrides?: SidebarTaskQueryOverrides,
) {
  const view = useEffectiveSidebarView(workspaceId);
  const { i18n } = useTranslation();
  const contextScope = useAppStore(sidebarTaskPageScope);
  const collapsedTaskIDs = useAppStore((state) => state.collapsedSubtaskParents);
  const workspaceGeneration = useAppStore((state) => state.workspaceContextGeneration);
  const queryRevision = useAppStore(
    (state) => state.sidebarArchivedTasks?.revisionByWorkspaceId?.[workspaceId ?? ""] ?? 0,
  );
  const rankingKey = useAppStore(sidebarTaskPageRankingKey);
  const revision = `${queryRevision}:${rankingKey}`;
  const accessDenied = useAppStore(
    (state) =>
      state.workspaceContextRead?.snapshotError === "access_denied" ||
      Object.values(state.workspaceContextRead?.errors ?? {}).includes("access_denied") ||
      Boolean(state.auth && state.auth.mode !== "disabled" && !state.auth.authenticated),
  );
  const prefs = useSidebarTaskPrefs();
  const locale = normalizeLocale(i18n.resolvedLanguage ?? i18n.language);
  const resultView = useMemo(
    () => resolveSidebarTaskView(view, queryOverrides),
    [view, queryOverrides?.filters, queryOverrides?.sort, queryOverrides?.group],
  );
  const queryView = useMemo(
    () => buildSidebarQuery(resultView, 1, locale, collapsedTaskIDs, queryOverrides?.query),
    [resultView, locale, collapsedTaskIDs, queryOverrides?.query],
  );
  const viewKey = useSidebarViewKey(
    workspaceId,
    workspaceGeneration,
    contextScope,
    queryView,
    prefs,
  );
  return {
    view,
    resultView,
    workspaceGeneration,
    revision,
    queryView,
    viewKey,
    prefs,
    accessDenied,
  };
}
