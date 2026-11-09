import type { TFunction } from "i18next";
import { isReadAbort } from "@/lib/api/temporary-read-error";
import {
  sidebarTaskPageCache,
  sidebarTaskPageRankingKey,
  sidebarTaskPageScope,
} from "@/lib/sidebar/sidebar-task-page-cache";
import {
  isSidebarTaskAccessDenied,
  sidebarTaskQueryError,
} from "@/lib/sidebar/sidebar-task-query-error";
import { isCurrentWorkspaceContext } from "@/lib/state/workspace-context";
import type { AppState } from "@/lib/state/store";
import type { SidebarTaskPageCache } from "@/lib/sidebar/sidebar-task-page-cache";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import type { SidebarPageLoadOptions } from "./sidebar-task-page-refresh";
import { sidebarContentKey } from "./use-sidebar-page-context";

export type SidebarPageStore = { getState: () => AppState };

export function queryRevision(store: SidebarPageStore, workspaceId: string) {
  return store.getState().sidebarArchivedTasks?.revisionByWorkspaceId?.[workspaceId] ?? 0;
}

function currentSidebarRequest(
  store: SidebarPageStore,
  workspaceId: string,
  validity: {
    request: boolean;
    view: boolean;
    workspaceGeneration: number;
    scope: string;
    rankingKey: string;
  },
) {
  const state = store.getState();
  return (
    validity.request &&
    validity.view &&
    isCurrentWorkspaceContext(state, workspaceId, validity.workspaceGeneration) &&
    sidebarTaskPageScope(state) === validity.scope &&
    sidebarTaskPageRankingKey(state) === validity.rankingKey
  );
}

function acceptedSidebarResponse(result: SidebarTaskPageResponse, revisionChanged: boolean) {
  return { ...result, provisional: result.provisional || revisionChanged };
}

export function createSidebarRequestCheck(args: {
  store: SidebarPageStore;
  workspaceId: string;
  generationRef: { current: number };
  generation: number;
  key: string;
  viewKeyRef: { current: string };
  workspaceGeneration: number;
  scope: string;
  rankingKey: string;
}) {
  return () =>
    currentSidebarRequest(args.store, args.workspaceId, {
      request: args.generationRef.current === args.generation,
      view: args.viewKeyRef.current === args.key,
      workspaceGeneration: args.workspaceGeneration,
      scope: args.scope,
      rankingKey: args.rankingKey,
    });
}

export function showCachedSidebarPage(args: {
  cached: SidebarTaskPageResponse | null;
  key: string;
  page: {
    setResponse: (page: SidebarTaskPageResponse) => void;
    setResponseViewKey: (key: string) => void;
    responseViewKeyRef: { current: string };
    pageNumberRef: { current: number };
    setPageNumber: (page: number) => void;
  };
}) {
  const { cached, key, page } = args;
  if (!cached || page.responseViewKeyRef.current === key) return;
  page.setResponse(cached);
  page.setResponseViewKey(key);
  page.responseViewKeyRef.current = key;
  page.pageNumberRef.current = 1;
  page.setPageNumber(1);
}

export function publishAcceptedSidebarPage(args: {
  result: SidebarTaskPageResponse;
  key: string;
  revisionChanged: boolean;
  page: {
    setResponse: (page: SidebarTaskPageResponse) => void;
    setResponseViewKey: (key: string) => void;
    responseViewKeyRef: { current: string };
    pageNumberRef: { current: number };
    setPageNumber: (page: number) => void;
    errorIdentityRef: { current: string | null };
    setError: (error: string | null) => void;
  };
}) {
  const { result, key, revisionChanged, page } = args;
  page.setResponse(acceptedSidebarResponse(result, revisionChanged));
  page.setResponseViewKey(key);
  page.responseViewKeyRef.current = key;
  page.pageNumberRef.current = result.page;
  page.setPageNumber(result.page);
  page.errorIdentityRef.current = null;
  page.setError(null);
}

export function publishSidebarLoadFailure(args: {
  store: SidebarPageStore;
  error: unknown;
  target: string;
  key: string;
  t: TFunction;
  page: {
    responseViewKeyRef: { current: string };
    errorIdentityRef: { current: string | null };
    setError: (error: string | null) => void;
    setCanRetry: (canRetry: boolean) => void;
  };
}): false {
  const { store, error, target, key, t, page } = args;
  if (isReadAbort(error)) return false;
  if (isSidebarTaskAccessDenied(error)) sidebarTaskPageCache(store).denyAccess();
  const hasDisplay = sidebarContentKey(page.responseViewKeyRef.current) === sidebarContentKey(key);
  const failure = sidebarTaskQueryError(error, hasDisplay, t);
  page.errorIdentityRef.current = target;
  page.setError(failure.message);
  page.setCanRetry(failure.canRetry);
  return false;
}

export type SidebarPageLoadState = {
  requestGeneration: { current: number };
  activeRequestRef: { current: ReturnType<SidebarTaskPageCache["request"]> | null };
  errorIdentityRef: { current: string | null };
  pendingLoads: { current: number };
  responseViewKeyRef: { current: string };
  pageNumberRef: { current: number };
  setResponse: (page: SidebarTaskPageResponse) => void;
  setResponseViewKey: (key: string) => void;
  setPageNumber: (page: number) => void;
  setPendingPage: (page: number | null) => void;
  setError: (error: string | null) => void;
  setCanRetry: (canRetry: boolean) => void;
};

type SidebarLoadContext = {
  request: ReturnType<SidebarTaskPageCache["request"]>;
  generation: number;
  startingRevision: number;
  isCurrent: () => boolean;
};

export function beginSidebarLoad(args: {
  state: SidebarPageLoadState;
  store: SidebarPageStore;
  viewKeyRef: { current: string };
  workspaceId: string;
  workspaceGeneration: number;
  requestedPage: number;
  key: string;
  queryBase: SidebarTaskQuery;
  options: SidebarPageLoadOptions;
}): SidebarLoadContext {
  const { state, store, viewKeyRef, workspaceId, requestedPage, key, queryBase, options } = args;
  const generation = ++state.requestGeneration.current;
  const previousRequest = state.activeRequestRef.current;
  const startingRevision = queryRevision(store, workspaceId);
  const startingState = store.getState();
  const startingScope = sidebarTaskPageScope(startingState);
  const startingRankingKey = sidebarTaskPageRankingKey(startingState);
  const isCurrent = createSidebarRequestCheck({
    store,
    workspaceId,
    generationRef: state.requestGeneration,
    generation,
    key,
    viewKeyRef,
    workspaceGeneration: args.workspaceGeneration,
    scope: startingScope,
    rankingKey: startingRankingKey,
  });
  const cached = requestedPage === 1 ? sidebarTaskPageCache(store).get(key) : null;
  showCachedSidebarPage({
    cached,
    key,
    page: state,
  });
  const target = JSON.stringify([key, requestedPage]);
  if (state.errorIdentityRef.current !== target) {
    state.setError(null);
    state.setCanRetry(true);
  }
  const request = sidebarTaskPageCache(store).request(
    workspaceId,
    { ...queryBase, page: requestedPage },
    key,
    {
      manualRetry: options.manualRetry,
      foregroundRecoveryEpisode: options.foregroundRecoveryEpisode,
    },
  );
  state.activeRequestRef.current = request;
  if (previousRequest) previousRequest.release();
  state.setPendingPage(requestedPage);
  state.pendingLoads.current += 1;
  return { request, generation, startingRevision, isCurrent };
}
