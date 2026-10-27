import { useEffect, type Dispatch, type SetStateAction } from "react";
import { sidebarTaskPageCache } from "@/lib/sidebar/sidebar-task-page-cache";
import type { SidebarTaskPrefs } from "@/lib/sidebar/apply-view";
import type { TaskOverview } from "@/lib/state/slices/task-overview-types";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import type { useAppStoreApi } from "@/components/state-provider";
import { useLocalSidebarPage } from "./use-local-sidebar-page";
import type { SidebarPageLoader } from "./use-sidebar-task-page";

type SidebarPageStore = ReturnType<typeof useAppStoreApi>;

export type SidebarPageAutoLoadOptions = {
  initialPage: number;
  workspaceId: string | null;
  workspaceGeneration: number;
  viewKey: string;
  queryView: SidebarTaskQuery;
  refreshRevision: number;
  setRefreshRevision: Dispatch<SetStateAction<number>>;
  loader: SidebarPageLoader;
  autoLoadKeyRef: { current: string };
  autoLoadScopeRef: { current: string };
  queuedRefreshRef: { current: boolean };
  refreshTimerRef: { current: ReturnType<typeof setTimeout> | null };
};

export function useSidebarPageAutoLoad({
  initialPage,
  workspaceId,
  workspaceGeneration,
  viewKey,
  queryView,
  refreshRevision,
  setRefreshRevision,
  loader,
  autoLoadKeyRef,
  autoLoadScopeRef,
  queuedRefreshRef,
  refreshTimerRef,
}: SidebarPageAutoLoadOptions): void {
  useEffect(
    () => () => {
      autoLoadKeyRef.current = "";
    },
    [],
  );

  useEffect(() => {
    if (!workspaceId) {
      loader.reset();
      autoLoadKeyRef.current = "";
      autoLoadScopeRef.current = "";
      queuedRefreshRef.current = false;
      return;
    }
    const autoLoadScope = `${workspaceId}:${workspaceGeneration}:${viewKey}`;
    if (autoLoadScopeRef.current !== autoLoadScope) {
      autoLoadScopeRef.current = autoLoadScope;
      queuedRefreshRef.current = false;
    }
    const autoLoadKey = `${workspaceId}:${workspaceGeneration}:${viewKey}:${refreshRevision}`;
    if (autoLoadKeyRef.current === autoLoadKey) return;
    autoLoadKeyRef.current = autoLoadKey;
    const sameView = loader.responseViewKeyRef.current === viewKey;
    const requestedPage = sameView ? loader.pageNumberRef.current : initialPage;
    void loader.loadPage(requestedPage, viewKey, queryView);
  }, [
    autoLoadKeyRef,
    autoLoadScopeRef,
    loader.loadPage,
    loader.pageNumberRef,
    loader.reset,
    loader.responseViewKeyRef,
    queryView,
    queuedRefreshRef,
    refreshRevision,
    viewKey,
    workspaceGeneration,
    workspaceId,
    initialPage,
  ]);

  useEffect(() => {
    if (loader.pendingPage !== null || !queuedRefreshRef.current) return;
    const timer = setTimeout(() => {
      if (!queuedRefreshRef.current) return;
      if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
      refreshTimerRef.current = null;
      queuedRefreshRef.current = false;
      setRefreshRevision((revision) => revision + 1);
    }, 250);
    return () => clearTimeout(timer);
  }, [loader.pendingPage, queuedRefreshRef, refreshTimerRef, setRefreshRevision]);
}

export function useSidebarPageLocalResponse(options: {
  enabled: boolean;
  store: SidebarPageStore;
  authorizedWorkspaceId: string | null;
  localTasks: TaskOverview[] | null;
  queryView: SidebarTaskQuery;
  viewKey: string;
  viewResponse: SidebarTaskPageResponse | null;
  prefs: SidebarTaskPrefs;
  isDisclosureTransition: boolean;
}) {
  const {
    enabled,
    store,
    authorizedWorkspaceId,
    localTasks,
    queryView,
    viewKey,
    viewResponse,
    prefs,
    isDisclosureTransition,
  } = options;
  useEffect(() => {
    if (!enabled) sidebarTaskPageCache(store).forget(viewKey);
  }, [enabled, store, viewKey]);
  const local = useLocalSidebarPage(
    authorizedWorkspaceId,
    queryView.query ? null : localTasks,
    queryView,
    viewKey,
    viewResponse?.page ?? 1,
    prefs,
  );
  return {
    local,
    currentResponse: local.response ?? viewResponse,
    disclosureTransition: !local.response && isDisclosureTransition,
  };
}
