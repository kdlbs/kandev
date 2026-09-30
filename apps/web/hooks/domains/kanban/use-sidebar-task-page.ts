import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
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
import { generateUUID } from "@/lib/utils";
import type { TaskOverview } from "@/lib/state/slices/task-overview-types";
import { useSidebarPageContext } from "./use-sidebar-page-context";
import { useLocalSidebarPage } from "./use-local-sidebar-page";

type SidebarPageStore = ReturnType<typeof useAppStoreApi>;

function useSidebarPageState(store: SidebarPageStore) {
  const [pendingPage, setPendingPage] = useState<number | null>(null);
  const [response, setPageResponse] = useState<SidebarTaskPageResponse | null>(null);
  const responseRef = useRef<SidebarTaskPageResponse | null>(null);
  const [owner] = useState(() => `sidebar:display:${generateUUID()}`);
  const setResponse = useCallback(
    (update: SetStateAction<SidebarTaskPageResponse | null>) => {
      const page = typeof update === "function" ? update(responseRef.current) : update;
      responseRef.current = page;
      const state = store.getState();
      const tasks =
        page?.entries.flatMap((entry) => {
          const task = entry.task_id ? state.taskOverview?.byId[entry.task_id] : undefined;
          return task ? [task] : [];
        }) ?? [];
      if (page) state.retainTaskOverviews?.(owner, tasks);
      else state.releaseTaskOverviews?.(owner);
      setPageResponse(page);
    },
    [store, owner],
  );
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
  }, [setResponse]);

  const hasInFlight = useCallback(() => activeRequestRef.current !== null, []);

  useEffect(
    () => () => {
      requestGeneration.current += 1;
      activeRequestRef.current?.release();
      activeRequestRef.current = null;
      store.getState().releaseTaskOverviews?.(owner);
    },
    [store, owner],
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

function currentSidebarRequest(
  store: SidebarPageStore,
  workspaceId: string,
  validity: { request: boolean; view: boolean; workspaceGeneration: number; scope: string },
) {
  return (
    validity.request &&
    validity.view &&
    isCurrentWorkspaceContext(store.getState(), workspaceId, validity.workspaceGeneration) &&
    sidebarTaskPageScope(store.getState()) === validity.scope
  );
}

function acceptedSidebarResponse(result: SidebarTaskPageResponse, revisionChanged: boolean) {
  return { ...result, provisional: result.provisional || revisionChanged };
}

function sidebarResponseState(
  enabled: boolean,
  loader: SidebarPageLoader,
  cached: SidebarTaskPageResponse | null,
  viewKey: string,
) {
  return {
    pendingPage: enabled ? loader.pendingPage : null,
    error: enabled ? loader.error : null,
    viewResponse: loader.responseViewKey === viewKey ? loader.response : cached,
  };
}

function useSidebarPageLoader(
  workspaceId: string | null,
  workspaceGeneration: number,
  store: SidebarPageStore,
  t: ReturnType<typeof useTranslation>["t"],
  viewKeyRef: { current: string },
) {
  const state = useSidebarPageState(store);
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
        currentSidebarRequest(store, workspaceId, {
          request: requestGeneration.current === generation,
          view: viewKeyRef.current === key,
          workspaceGeneration: startingWorkspaceGeneration,
          scope: startingScope,
        });
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
      setCanRetry(true);
      const request = sidebarTaskPageCache(store).request(
        workspaceId,
        { ...queryBase, page: requestedPage },
        key,
      );
      activeRequestRef.current = request;
      try {
        const result = await request.promise;
        if (!isCurrent()) return false;
        setResponse(
          acceptedSidebarResponse(result, queryRevision(store, workspaceId) !== startingRevision),
        );
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
    [store, t, workspaceGeneration, workspaceId, viewKeyRef, setResponse],
  );

  return { ...state, loadPage };
}

function useSidebarRevisionRefresh(
  workspaceId: string | null,
  queryRevision: number,
  refresh: () => void,
  refreshTimerRef: { current: ReturnType<typeof setTimeout> | null },
) {
  const seenQueryRevisionRef = useRef<{ workspaceId: string; revision: number } | null>(null);
  const refreshBurstStartedRef = useRef<number | null>(null);

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
  }, [queryRevision, refresh, workspaceId, refreshTimerRef]);

  useEffect(
    () => () => {
      if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
    },
    [refreshTimerRef],
  );
  return refreshTimerRef;
}

function sidebarLoadingState(
  enabled: boolean,
  response: SidebarTaskPageResponse | null,
  pending: number | null,
) {
  const emptyProvisional =
    response?.provisional === true && !response.entries.some((row) => row.kind === "task");
  return {
    isLoading: enabled && (emptyProvisional || (pending !== null && response === null)),
    isRefreshing:
      enabled && response !== null && (pending !== null || response.provisional === true),
  };
}

function useProvisionalSidebarRefresh(
  workspaceId: string | null,
  response: SidebarTaskPageResponse | null,
  pending: number | null,
  options: { refresh: () => void; isRefreshScheduled: () => boolean },
) {
  const { refresh, isRefreshScheduled } = options;
  useEffect(() => {
    if (!workspaceId || !response?.provisional || pending !== null || isRefreshScheduled()) return;
    const timer = setTimeout(refresh, 250);
    return () => clearTimeout(timer);
  }, [workspaceId, response, pending, refresh, isRefreshScheduled]);
}

function useSidebarLiveRefresh(
  workspaceId: string | null,
  revision: number,
  page: {
    response: SidebarTaskPageResponse | null;
    pending: number | null;
    queued: { current: boolean };
    timer: { current: ReturnType<typeof setTimeout> | null };
  },
  refresh: () => void,
) {
  const timer = useSidebarRevisionRefresh(workspaceId, revision, refresh, page.timer);
  const { queued } = page;
  const isRefreshScheduled = useCallback(
    () => queued.current || timer.current !== null,
    [queued, timer],
  );
  useProvisionalSidebarRefresh(workspaceId, page.response, page.pending, {
    refresh,
    isRefreshScheduled,
  });
}

type SidebarPageLoader = ReturnType<typeof useSidebarPageLoader>;

type SidebarPageAutoLoadOptions = {
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

function useSidebarPageAutoLoad({
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

function useSidebarDeletedTasks(
  store: SidebarPageStore,
  loader: SidebarPageLoader,
  viewKey: string,
  queryView: SidebarTaskQuery,
) {
  const { setResponse, loadPage, pageNumberRef } = loader;
  useEffect(
    () =>
      sidebarTaskPageCache(store).subscribeDeletedTasks((taskIds) => {
        setResponse((current) =>
          current
            ? {
                ...current,
                entries: current.entries.filter(
                  (entry) => !entry.task_id || !taskIds.has(entry.task_id),
                ),
              }
            : current,
        );
        // Starting the replacement also fences any pre-deletion response still in flight.
        void loadPage(pageNumberRef.current, viewKey, queryView);
      }),
    [store, setResponse, loadPage, pageNumberRef, viewKey, queryView],
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
    if (pending.key !== viewKey || error || (!currentResponse && pendingPage === null)) {
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

function useSidebarPageRefresh(
  hasInFlight: () => boolean,
  queued: { current: boolean },
  timer: { current: ReturnType<typeof setTimeout> | null },
  setRefreshRevision: Dispatch<SetStateAction<number>>,
) {
  return useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
    if (hasInFlight()) {
      queued.current = true;
      return;
    }
    queued.current = false;
    setRefreshRevision((revision) => revision + 1);
  }, [hasInFlight, queued, timer, setRefreshRevision]);
}

/** Covered views page locally; incomplete views retain one bounded server page. */
export function useSidebarTaskPage(
  workspaceId: string | null,
  enabled = true,
  localTasks: TaskOverview[] | null = null,
) {
  const { view, workspaceGeneration, revision, queryView, viewKey, prefs, accessDenied } =
    useSidebarPageContext(workspaceId);
  const { pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId } = prefs;
  const { t } = useTranslation();
  const store = useAppStoreApi();
  const [refreshRevision, setRefreshRevision] = useState(0);
  const viewKeyRef = useRef("");
  const autoLoadKeyRef = useRef("");
  const autoLoadScopeRef = useRef("");
  const queued = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  viewKeyRef.current = viewKey;
  const queryWorkspaceId = enabled && !accessDenied ? workspaceId : null;
  const loader = useSidebarPageLoader(queryWorkspaceId, workspaceGeneration, store, t, viewKeyRef);
  const { loadPage } = loader;
  useSidebarDeletedTasks(store, loader, viewKey, queryView);
  const cachedResponse = queryWorkspaceId ? sidebarTaskPageCache(store).get(viewKey) : null;
  const {
    pendingPage,
    error: loadError,
    viewResponse,
  } = sidebarResponseState(enabled, loader, cachedResponse, viewKey);
  const error = accessDenied ? t("sidebar:workspaceContextAccessDenied") : loadError;
  const local = useLocalSidebarPage(
    workspaceId,
    localTasks,
    queryView,
    viewKey,
    viewResponse?.page ?? 1,
    {
      pinnedTaskIds,
      orderedTaskIds,
      subtaskOrderByParentId,
    },
  );
  const currentResponse = local.response ?? (enabled ? viewResponse : null);

  useEffect(() => {
    if (!enabled) sidebarTaskPageCache(store).forget(viewKey);
  }, [enabled, store, viewKey]);

  useSidebarPageAutoLoad({
    initialPage: local.previousPage,
    workspaceId: queryWorkspaceId,
    workspaceGeneration,
    viewKey,
    queryView,
    refreshRevision,
    setRefreshRevision,
    loader,
    autoLoadKeyRef,
    autoLoadScopeRef,
    queuedRefreshRef: queued,
    refreshTimerRef: timer,
  });

  const refresh = useSidebarPageRefresh(loader.hasInFlight, queued, timer, setRefreshRevision);

  useForegroundRefresh(refresh, Boolean(queryWorkspaceId), queryWorkspaceId);
  useSidebarLiveRefresh(
    queryWorkspaceId,
    revision,
    {
      response: currentResponse,
      pending: pendingPage,
      queued: queued,
      timer,
    },
    refresh,
  );

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
    page: currentResponse?.page ?? 1,
    requestedPage: pendingPage,
    ...sidebarLoadingState(Boolean(queryWorkspaceId), currentResponse, pendingPage),
    error,
    hasError: error !== null,
    canRetry: !accessDenied && loader.canRetry,
    refresh,
    retry,
    goToPage: local.response ? local.goToPage : goToPage,
    scopeKey: viewKey,
    view,
    pinnedTaskIds,
    orderedTaskIds,
    subtaskOrderByParentId,
  };
}
