import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useTranslation } from "react-i18next";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  sidebarTaskPageCache,
  sidebarTaskPageScope,
  type SidebarTaskPageCache,
} from "@/lib/sidebar/sidebar-task-page-cache";
import {
  sidebarTaskQueryError,
  isSidebarTaskAccessDenied,
} from "@/lib/sidebar/sidebar-task-query-error";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { isCurrentWorkspaceContext } from "@/lib/state/workspace-context";
import { normalizeLocale } from "@/lib/i18n";

const PAGE_SIZE = 100;
function buildSidebarQuery(
  view: ReturnType<typeof useEffectiveSidebarView>,
  page: number,
  locale: string,
  collapsedTaskIDs: string[],
): SidebarTaskQuery {
  return {
    filters: view.filters.map(({ dimension, op, value }) => ({ dimension, op, value })),
    sort: view.sort,
    group: view.group,
    collapsed_group_keys: view.collapsedGroups,
    collapsed_task_ids: collapsedTaskIDs,
    page,
    page_size: PAGE_SIZE,
    locale,
  };
}

type SidebarPageStore = ReturnType<typeof useAppStoreApi>;

function useSidebarPageState() {
  const [pendingPage, setPendingPage] = useState<number | null>(null);
  const [response, setResponse] = useState<SidebarTaskPageResponse | null>(null);
  const [responseViewKey, setResponseViewKey] = useState("");
  const [pageNumber, setPageNumber] = useState(1);
  const [error, setError] = useState<string | null>(null);
  const [canRetry, setCanRetry] = useState(true);
  const requestGeneration = useRef(0);
  const activeRequestRef = useRef<ReturnType<SidebarTaskPageCache["request"]> | null>(null);
  const responseViewKeyRef = useRef("");
  const pageNumberRef = useRef(1);

  const reset = useCallback(() => {
    requestGeneration.current += 1;
    activeRequestRef.current?.release();
    activeRequestRef.current = null;
    setResponse(null);
    setResponseViewKey("");
    responseViewKeyRef.current = "";
    setPageNumber(1);
    pageNumberRef.current = 1;
    setPendingPage(null);
    setError(null);
  }, []);

  const hasInFlight = useCallback(() => activeRequestRef.current !== null, []);

  useEffect(
    () => () => {
      requestGeneration.current += 1;
      activeRequestRef.current?.release();
      activeRequestRef.current = null;
    },
    [],
  );

  return {
    response,
    setResponse,
    responseViewKey,
    setResponseViewKey,
    responseViewKeyRef,
    pageNumber,
    setPageNumber,
    pageNumberRef,
    pendingPage,
    setPendingPage,
    error,
    setError,
    canRetry,
    setCanRetry,
    requestGeneration,
    activeRequestRef,
    reset,
    hasInFlight,
  };
}

function useSidebarAccessDenial(
  store: SidebarPageStore,
  state: ReturnType<typeof useSidebarPageState>,
  t: ReturnType<typeof useTranslation>["t"],
) {
  const { reset, setError, setCanRetry } = state;
  useEffect(
    () =>
      sidebarTaskPageCache(store).subscribeAccessDenied(() => {
        reset();
        setError(t("sidebar:workspaceContextAccessDenied"));
        setCanRetry(false);
      }),
    [store, reset, setError, setCanRetry, t],
  );
}

function queryRevision(store: SidebarPageStore, workspaceId: string) {
  return store.getState().sidebarArchivedTasks?.revisionByWorkspaceId?.[workspaceId] ?? 0;
}

function useSidebarPageLoader(
  workspaceId: string | null,
  workspaceGeneration: number,
  store: SidebarPageStore,
  t: ReturnType<typeof useTranslation>["t"],
  viewKeyRef: { current: string },
) {
  const state = useSidebarPageState();
  useSidebarAccessDenial(store, state, t);
  const {
    setResponse,
    setResponseViewKey,
    responseViewKeyRef,
    setPageNumber,
    pageNumberRef,
    setPendingPage,
    setError,
    setCanRetry,
    requestGeneration,
    activeRequestRef,
  } = state;

  const loadPage = useCallback(
    async (requestedPage: number, key: string, queryBase: SidebarTaskQuery): Promise<boolean> => {
      if (!workspaceId) return false;
      const generation = ++requestGeneration.current;
      activeRequestRef.current?.release();
      activeRequestRef.current = null;
      const startingWorkspaceGeneration = workspaceGeneration;
      const startingRevision = queryRevision(store, workspaceId);
      const startingScope = sidebarTaskPageScope(store.getState());
      const isCurrent = () =>
        requestGeneration.current === generation &&
        viewKeyRef.current === key &&
        isCurrentWorkspaceContext(store.getState(), workspaceId, startingWorkspaceGeneration) &&
        sidebarTaskPageScope(store.getState()) === startingScope;
      const cached = requestedPage === 1 ? sidebarTaskPageCache(store).get(key) : null;
      if (cached && responseViewKeyRef.current !== key) {
        setResponse(cached);
        setResponseViewKey(key);
        responseViewKeyRef.current = key;
        pageNumberRef.current = 1;
        setPageNumber(1);
      }
      setPendingPage(requestedPage);
      setError(null);
      const request = sidebarTaskPageCache(store).request(
        workspaceId,
        { ...queryBase, page: requestedPage },
        key,
      );
      activeRequestRef.current = request;
      try {
        const result = await request.promise;
        if (!isCurrent()) return false;
        if (queryRevision(store, workspaceId) !== startingRevision) {
          pageNumberRef.current = requestedPage;
          return false;
        }
        setResponse(result);
        setResponseViewKey(key);
        responseViewKeyRef.current = key;
        pageNumberRef.current = result.page;
        setPageNumber(result.page);
        setError(null);
        return true;
      } catch (loadError) {
        if (!isCurrent()) return false;
        if (loadError instanceof DOMException && loadError.name === "AbortError") return false;
        if (isSidebarTaskAccessDenied(loadError)) {
          sidebarTaskPageCache(store).denyAccess();
        }
        const failure = sidebarTaskQueryError(loadError, responseViewKeyRef.current === key, t);
        setError(failure.message);
        setCanRetry(failure.canRetry);
        return false;
      } finally {
        request.release();
        if (activeRequestRef.current === request) activeRequestRef.current = null;
        if (requestGeneration.current === generation) setPendingPage(null);
      }
    },
    [store, t, workspaceGeneration, workspaceId, viewKeyRef],
  );

  return { ...state, loadPage };
}

function useSidebarRevisionRefresh(
  workspaceId: string | null,
  queryRevision: number,
  refresh: () => void,
): void {
  const seenQueryRevisionRef = useRef<{ workspaceId: string; revision: number } | null>(null);
  const refreshBurstStartedRef = useRef<number | null>(null);
  const refreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!workspaceId) {
      seenQueryRevisionRef.current = null;
      refreshBurstStartedRef.current = null;
      if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
      refreshTimerRef.current = null;
      return;
    }
    const previousRevision = seenQueryRevisionRef.current;
    if (!previousRevision || previousRevision.workspaceId !== workspaceId) {
      seenQueryRevisionRef.current = { workspaceId, revision: queryRevision };
      refreshBurstStartedRef.current = null;
      if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
      refreshTimerRef.current = null;
      return;
    }
    if (previousRevision.revision === queryRevision) return;
    seenQueryRevisionRef.current = { workspaceId, revision: queryRevision };
    const now = Date.now();
    refreshBurstStartedRef.current ??= now;
    if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
    const elapsed = now - refreshBurstStartedRef.current;
    const delay = Math.max(0, Math.min(250, 2000 - elapsed));
    refreshTimerRef.current = setTimeout(() => {
      refreshTimerRef.current = null;
      refreshBurstStartedRef.current = null;
      refresh();
    }, delay);
    return () => {
      if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
      refreshTimerRef.current = null;
    };
  }, [queryRevision, refresh, workspaceId]);

  useEffect(
    () => () => {
      if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
    },
    [],
  );
}

type SidebarPageLoader = ReturnType<typeof useSidebarPageLoader>;

type SidebarPageAutoLoadOptions = {
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
};

function useSidebarPageAutoLoad({
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
    const requestedPage = sameView ? loader.pageNumberRef.current : 1;
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
  ]);

  useEffect(() => {
    if (loader.pendingPage !== null || !queuedRefreshRef.current) return;
    queuedRefreshRef.current = false;
    setRefreshRevision((revision) => revision + 1);
  }, [loader.pendingPage, queuedRefreshRef, setRefreshRevision]);
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

function useSidebarPageNavigation({
  currentResponse,
  pendingPage,
  error,
  loadPage,
  viewKey,
  queryView,
}: {
  currentResponse: SidebarTaskPageResponse | null;
  pendingPage: number | null;
  error: string | null;
  loadPage: SidebarPageLoader["loadPage"];
  viewKey: string;
  queryView: SidebarTaskQuery;
}) {
  const navigation = useRef<{
    key: string;
    previous: SidebarTaskPageResponse;
    afterSuccess: () => void;
  } | null>(null);
  useEffect(() => {
    const pending = navigation.current;
    if (!pending) return;
    if (pending.key !== viewKey || error) {
      navigation.current = null;
      return;
    }
    if (pendingPage !== null || !currentResponse || currentResponse === pending.previous) return;
    navigation.current = null;
    requestAnimationFrame(pending.afterSuccess);
  }, [currentResponse, error, pendingPage, viewKey]);
  return useCallback(
    (requestedPage: number, afterSuccess?: () => void) => {
      if (!currentResponse || pendingPage !== null || requestedPage < 1) return;
      if (requestedPage > currentResponse.page + (currentResponse.has_next ? 1 : 0)) return;
      if (requestedPage < currentResponse.page && !currentResponse.has_previous) return;
      if (requestedPage === currentResponse.page) return;
      navigation.current = afterSuccess
        ? { key: viewKey, previous: currentResponse, afterSuccess }
        : null;
      void loadPage(requestedPage, viewKey, queryView);
    },
    [currentResponse, loadPage, pendingPage, queryView, viewKey],
  );
}

/** One current server-ordered page is shared by every built-in, saved, and draft view. */
export function useSidebarTaskPage(workspaceId: string | null) {
  const view = useEffectiveSidebarView(workspaceId);
  const { i18n, t } = useTranslation();
  const store = useAppStoreApi();
  const contextScope = useAppStore(sidebarTaskPageScope);
  const collapsedTaskIDs = useAppStore((state) => state.collapsedSubtaskParents);
  const workspaceGeneration = useAppStore((state) => state.workspaceContextGeneration);
  const queryRevision = useAppStore(
    (state) => state.sidebarArchivedTasks?.revisionByWorkspaceId?.[workspaceId ?? ""] ?? 0,
  );
  const { pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId } = useSidebarTaskPrefs();
  const [refreshRevision, setRefreshRevision] = useState(0);
  const viewKeyRef = useRef("");
  const autoLoadKeyRef = useRef("");
  const autoLoadScopeRef = useRef("");
  const queuedRefreshRef = useRef(false);

  const locale = normalizeLocale(i18n.resolvedLanguage ?? i18n.language);
  const queryView = useMemo(
    () => buildSidebarQuery(view, 1, locale, collapsedTaskIDs),
    [view, locale, collapsedTaskIDs],
  );
  const viewKey = useSidebarViewKey(workspaceId, workspaceGeneration, contextScope, queryView, {
    pinnedTaskIds,
    orderedTaskIds,
    subtaskOrderByParentId,
  });
  viewKeyRef.current = viewKey;
  const loader = useSidebarPageLoader(workspaceId, workspaceGeneration, store, t, viewKeyRef);
  const { response, pendingPage, error, loadPage } = loader;
  const cachedResponse = workspaceId ? sidebarTaskPageCache(store).get(viewKey) : null;
  const currentResponse = loader.responseViewKey === viewKey ? response : cachedResponse;
  const currentPage = currentResponse?.page ?? 1;

  useSidebarPageAutoLoad({
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
  });

  const refresh = useCallback(() => {
    if (loader.hasInFlight()) {
      queuedRefreshRef.current = true;
      return;
    }
    queuedRefreshRef.current = false;
    setRefreshRevision((revision) => revision + 1);
  }, [loader.hasInFlight]);

  useForegroundRefresh(refresh, Boolean(workspaceId), workspaceId);
  useSidebarRevisionRefresh(workspaceId, queryRevision, refresh);

  const goToPage = useSidebarPageNavigation({
    currentResponse,
    pendingPage,
    error,
    loadPage,
    viewKey,
    queryView,
  });

  const retry = useCallback(() => {
    void loadPage(currentResponse?.page ?? 1, viewKey, queryView);
  }, [currentResponse, loadPage, queryView, viewKey]);

  return {
    response: currentResponse,
    page: currentPage,
    requestedPage: pendingPage,
    isLoading: pendingPage !== null && currentResponse === null,
    isRefreshing: pendingPage !== null && currentResponse !== null,
    error,
    hasError: error !== null,
    canRetry: loader.canRetry,
    refresh,
    retry,
    goToPage,
    scopeKey: viewKey,
    view,
    pinnedTaskIds,
    orderedTaskIds,
    subtaskOrderByParentId,
  };
}
