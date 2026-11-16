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
import { sidebarSortToWire } from "@/lib/sidebar/sidebar-sort-chain";

import { usePluginRegistry } from "@/lib/plugins/registry";
const PAGE_SIZE = 100;
function buildSidebarQuery(
  view: ReturnType<typeof useEffectiveSidebarView>,
  page: number,
  locale: string,
  collapsedTaskIDs: string[],
): SidebarTaskQuery {
  return {
    filters: view.filters.map(({ dimension, op, value }) => ({ dimension, op, value })),
    sort: sidebarSortToWire(view.sort),
    group: view.group,
    collapsed_group_keys: view.collapsedGroups,
    collapsed_task_ids: collapsedTaskIDs,
    page,
    page_size: PAGE_SIZE,
    locale,
  };
}

function useSidebarViewKey(
  context: {
    workspaceId: string | null;
    workspaceGeneration: number;
    contextScope: string;
    groupingIdentity: string;
  },
  queryView: SidebarTaskQuery,
  prefs: Pick<
    ReturnType<typeof useSidebarTaskPrefs>,
    "pinnedTaskIds" | "orderedTaskIds" | "subtaskOrderByParentId"
  >,
) {
  const { workspaceId, workspaceGeneration, contextScope, groupingIdentity } = context;
  const { pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId } = prefs;
  return useMemo(
    () =>
      JSON.stringify({
        workspaceId,
        workspaceGeneration,
        contextScope,
        ...queryView,
        groupingIdentity,
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
      groupingIdentity,
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

export function useSidebarPageContext(workspaceId: string | null) {
  const view = useEffectiveSidebarView(workspaceId);
  const registry = usePluginRegistry();
  const groupingIdentity = registry.sidebarGroupingIdentity(view.group, workspaceId);
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
  const queryView = useMemo(
    () => buildSidebarQuery(view, 1, locale, collapsedTaskIDs),
    [view, locale, collapsedTaskIDs],
  );
  const viewKey = useSidebarViewKey(
    { workspaceId, workspaceGeneration, contextScope, groupingIdentity },
    queryView,
    prefs,
  );
  return { view, workspaceGeneration, revision, queryView, viewKey, prefs, accessDenied };
}
