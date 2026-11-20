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
  type SidebarTaskPageCache,
} from "@/lib/sidebar/sidebar-task-page-cache";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { generateUUID } from "@/lib/utils";
import type { TaskOverview } from "@/lib/state/slices/task-overview-types";
import { useSidebarPageContext } from "./use-sidebar-page-context";
import { sidebarLoadingState, sidebarResponseState } from "./sidebar-task-page-display";
import {
  useSidebarLiveRefresh,
  useSidebarPageAutoLoad,
  useSidebarPageRefresh,
  type SidebarPageLoadOptions,
  type SidebarPageRefreshTrigger,
} from "./sidebar-task-page-refresh";
import { useLocalSidebarPage } from "./use-local-sidebar-page";
import {
  beginSidebarLoad,
  publishAcceptedSidebarPage,
  publishSidebarLoadFailure,
  queryRevision,
  type SidebarPageStore,
} from "./sidebar-task-page-load";

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
  const pendingLoads = useRef(0);
  const errorIdentityRef = useRef<string | null>(null);

  const reset = useCallback(() => {
    requestGeneration.current += 1;
    activeRequestRef.current?.release();
    activeRequestRef.current = null;
    errorIdentityRef.current = null;
    setResponse(null);
    setResponseViewKey("");
    responseViewKeyRef.current = "";
    setPageNumber(1);
    pageNumberRef.current = 1;
    setPendingPage(null);
    setError(null);
  }, [setResponse]);

  const hasInFlight = useCallback(() => pendingLoads.current > 0, []);

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
    pendingLoads,
    error,
    setError,
    errorIdentityRef,
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

function useSidebarPageLoader(
  workspaceId: string | null,
  workspaceGeneration: number,
  store: SidebarPageStore,
  t: ReturnType<typeof useTranslation>["t"],
  viewKeyRef: { current: string },
) {
  const state = useSidebarPageState(store);
  useSidebarAccessDenial(store, state, t);

  const loadPage = useCallback(
    async (
      requestedPage: number,
      key: string,
      queryBase: SidebarTaskQuery,
      options: SidebarPageLoadOptions = {},
    ): Promise<boolean> => {
      if (!workspaceId) return false;
      const load = beginSidebarLoad({
        state,
        store,
        viewKeyRef,
        workspaceId,
        workspaceGeneration,
        requestedPage,
        key,
        queryBase,
        options,
      });
      try {
        const result = await load.request.promise;
        if (!load.isCurrent()) return false;
        publishAcceptedSidebarPage({
          result,
          key,
          revisionChanged: queryRevision(store, workspaceId) !== load.startingRevision,
          page: state,
        });
        return true;
      } catch (loadError) {
        if (!load.isCurrent()) return false;
        return publishSidebarLoadFailure({
          store,
          error: loadError,
          target: JSON.stringify([key, requestedPage]),
          key,
          t,
          page: state,
        });
      } finally {
        state.pendingLoads.current -= 1;
        if (state.requestGeneration.current === load.generation) state.setPendingPage(null);
      }
    },
    [store, t, workspaceGeneration, workspaceId, viewKeyRef, state],
  );

  return { ...state, loadPage };
}

type SidebarPageLoader = ReturnType<typeof useSidebarPageLoader>;

function useSidebarDeletedTasks(store: SidebarPageStore, loader: SidebarPageLoader) {
  const { setResponse } = loader;
  useEffect(
    () =>
      sidebarTaskPageCache(store).subscribeDeletedTasks((taskIds) => {
        setResponse((current) =>
          current
            ? {
                ...current,
                provisional: true,
                entries: current.entries.filter(
                  (entry) => !entry.task_id || !taskIds.has(entry.task_id),
                ),
              }
            : current,
        );
      }),
    [store, setResponse],
  );
}

function useSidebarPageNavigation({
  currentResponse,
  pendingPage,
  error,
  loadPage,
  viewKey,
  queryView,
  disclosureTransition,
}: {
  currentResponse: SidebarTaskPageResponse | null;
  pendingPage: number | null;
  error: string | null;
  loadPage: SidebarPageLoader["loadPage"];
  viewKey: string;
  queryView: SidebarTaskQuery;
  disclosureTransition: boolean;
}) {
  const retry = useCallback(() => {
    void loadPage(disclosureTransition ? 1 : (currentResponse?.page ?? 1), viewKey, queryView, {
      manualRetry: true,
    });
  }, [currentResponse, disclosureTransition, loadPage, queryView, viewKey]);
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
  const goToPage = useCallback(
    (requestedPage: number, afterSuccess?: () => void) => {
      if (disclosureTransition || !currentResponse || pendingPage !== null || requestedPage < 1)
        return;
      if (requestedPage > currentResponse.page + (currentResponse.has_next ? 1 : 0)) return;
      if (requestedPage < currentResponse.page && !currentResponse.has_previous) return;
      if (requestedPage === currentResponse.page) return;
      navigation.current = afterSuccess
        ? { key: viewKey, previous: currentResponse, afterSuccess }
        : null;
      void loadPage(requestedPage, viewKey, queryView, { manualRetry: true });
    },
    [currentResponse, disclosureTransition, loadPage, pendingPage, queryView, viewKey],
  );
  return { goToPage, retry };
}

function useSidebarPageDisplay(
  store: SidebarPageStore,
  queryWorkspaceId: string | null,
  loader: ReturnType<typeof useSidebarPageLoader>,
  viewKey: string,
) {
  const cached = queryWorkspaceId ? sidebarTaskPageCache(store).get(viewKey) : null;
  return sidebarResponseState(Boolean(queryWorkspaceId), loader, cached, viewKey);
}

function useSidebarRefreshControls(args: {
  queryWorkspaceId: string | null;
  workspaceGeneration: number;
  viewKey: string;
  queryView: SidebarTaskQuery;
  revision: string;
  refreshRevision: number;
  setRefreshRevision: Dispatch<SetStateAction<number>>;
  initialPage: number;
  loader: ReturnType<typeof useSidebarPageLoader>;
  currentResponse: SidebarTaskPageResponse | null;
  error: string | null;
  activeRequestRef: { current: ReturnType<SidebarTaskPageCache["request"]> | null };
  refs: {
    autoLoadKey: { current: string };
    autoLoadScope: { current: string };
    queued: { current: boolean };
    timer: { current: ReturnType<typeof setTimeout> | null };
    trigger: { current: SidebarPageRefreshTrigger };
  };
}) {
  const { autoLoadKey: autoLoadKeyRef, autoLoadScope: autoLoadScopeRef } = args.refs;
  const { queued, timer, trigger: refreshTrigger } = args.refs;
  const claimForegroundEpisode = useCallback(
    (episode: object) => {
      args.activeRequestRef.current?.claimForegroundEpisode(episode);
    },
    [args.activeRequestRef],
  );
  useSidebarPageAutoLoad({
    initialPage: args.initialPage,
    workspaceId: args.queryWorkspaceId,
    workspaceGeneration: args.workspaceGeneration,
    viewKey: args.viewKey,
    queryView: args.queryView,
    refreshRevision: args.refreshRevision,
    setRefreshRevision: args.setRefreshRevision,
    loader: args.loader,
    autoLoadKeyRef,
    autoLoadScopeRef,
    queuedRefreshRef: queued,
    refreshTimerRef: timer,
    refreshTriggerRef: refreshTrigger,
    hasTerminalError: args.error !== null,
  });
  const refresh = useSidebarPageRefresh({
    hasInFlight: args.loader.hasInFlight,
    queued,
    timer,
    setRefreshRevision: args.setRefreshRevision,
    refreshTriggerRef: refreshTrigger,
    claimForegroundEpisode,
  });
  useForegroundRefresh(
    () => refresh("foreground"),
    Boolean(args.queryWorkspaceId),
    args.queryWorkspaceId,
  );
  useSidebarLiveRefresh(
    args.queryWorkspaceId,
    args.revision,
    {
      response: args.currentResponse,
      pending: args.loader.pendingPage,
      error: args.error,
      queued,
      timer,
    },
    refresh,
  );
  return refresh;
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
  const refreshTrigger = useRef<SidebarPageRefreshTrigger>({ kind: "background" });

  viewKeyRef.current = viewKey;
  const authorizedWorkspaceId = accessDenied ? null : workspaceId;
  const queryWorkspaceId = enabled ? authorizedWorkspaceId : null;
  const loader = useSidebarPageLoader(queryWorkspaceId, workspaceGeneration, store, t, viewKeyRef);
  const { loadPage } = loader;
  useSidebarDeletedTasks(store, loader);
  const {
    pendingPage,
    error: loadError,
    viewResponse,
    isDisclosureTransition,
  } = useSidebarPageDisplay(store, queryWorkspaceId, loader, viewKey);
  const error = accessDenied ? t("sidebar:workspaceContextAccessDenied") : loadError;
  const local = useLocalSidebarPage(
    authorizedWorkspaceId,
    localTasks,
    queryView,
    viewKey,
    viewResponse?.page ?? 1,
    prefs,
  );
  const currentResponse = local.response ?? viewResponse;
  const disclosureTransition = !local.response && isDisclosureTransition;

  useEffect(() => {
    if (!enabled) sidebarTaskPageCache(store).forget(viewKey);
  }, [enabled, store, viewKey]);

  const refresh = useSidebarRefreshControls({
    queryWorkspaceId,
    workspaceGeneration,
    viewKey,
    queryView,
    revision,
    refreshRevision,
    setRefreshRevision,
    initialPage: local.previousPage,
    loader,
    currentResponse,
    error,
    activeRequestRef: loader.activeRequestRef,
    refs: {
      autoLoadKey: autoLoadKeyRef,
      autoLoadScope: autoLoadScopeRef,
      queued,
      timer,
      trigger: refreshTrigger,
    },
  });

  const { goToPage, retry } = useSidebarPageNavigation({
    currentResponse,
    disclosureTransition,
    pendingPage,
    error,
    loadPage,
    viewKey,
    queryView,
  });

  return {
    response: currentResponse,
    page: currentResponse?.page ?? 1,
    requestedPage: pendingPage,
    isDisclosureTransition: disclosureTransition,
    ...sidebarLoadingState(Boolean(queryWorkspaceId), currentResponse, pendingPage, {
      collapsedGroups: queryView.collapsed_group_keys,
      transitioning: disclosureTransition,
    }),
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
