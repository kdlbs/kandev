import { useCallback, useEffect, useRef, type Dispatch, type SetStateAction } from "react";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";

export type SidebarPageLoadOptions = {
  manualRetry?: boolean;
  foregroundRecoveryEpisode?: object;
};

export type SidebarPageRefreshTrigger = {
  kind: "background" | "foreground";
  episode?: object;
};

export type SidebarPageAutoLoadLoader = {
  loadPage: (
    page: number,
    key: string,
    query: SidebarTaskQuery,
    options?: SidebarPageLoadOptions,
  ) => Promise<boolean>;
  reset: () => void;
  pendingPage: number | null;
  pageNumberRef: { current: number };
  responseViewKeyRef: { current: string };
};

function useSidebarRevisionRefresh(
  workspaceId: string | null,
  queryRevision: string,
  refresh: () => void,
  refreshTimerRef: { current: ReturnType<typeof setTimeout> | null },
) {
  const seenQueryRevisionRef = useRef<{ workspaceId: string; revision: string } | null>(null);
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

function useProvisionalSidebarRefresh(
  workspaceId: string | null,
  response: SidebarTaskPageResponse | null,
  pending: number | null,
  error: string | null,
  options: { refresh: () => void; isRefreshScheduled: () => boolean },
) {
  const { refresh, isRefreshScheduled } = options;
  useEffect(() => {
    if (
      !workspaceId ||
      !response?.provisional ||
      pending !== null ||
      error !== null ||
      isRefreshScheduled()
    )
      return;
    const timer = setTimeout(refresh, 250);
    return () => clearTimeout(timer);
  }, [workspaceId, response, pending, error, refresh, isRefreshScheduled]);
}

export function useSidebarLiveRefresh(
  workspaceId: string | null,
  revision: string,
  page: {
    response: SidebarTaskPageResponse | null;
    pending: number | null;
    error: string | null;
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
  useProvisionalSidebarRefresh(workspaceId, page.response, page.pending, page.error, {
    refresh,
    isRefreshScheduled,
  });
}

type SidebarPageAutoLoadOptions = {
  initialPage: number;
  workspaceId: string | null;
  workspaceGeneration: number;
  viewKey: string;
  queryView: SidebarTaskQuery;
  refreshRevision: number;
  setRefreshRevision: Dispatch<SetStateAction<number>>;
  loader: SidebarPageAutoLoadLoader & { pendingPage: number | null };
  autoLoadKeyRef: { current: string };
  autoLoadScopeRef: { current: string };
  queuedRefreshRef: { current: boolean };
  refreshTimerRef: { current: ReturnType<typeof setTimeout> | null };
  refreshTriggerRef: { current: SidebarPageRefreshTrigger };
  hasTerminalError: boolean;
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
  refreshTriggerRef,
  hasTerminalError,
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
    const trigger = refreshTriggerRef.current;
    refreshTriggerRef.current = { kind: "background" };
    void loader.loadPage(requestedPage, viewKey, queryView, {
      foregroundRecoveryEpisode: trigger.kind === "foreground" ? trigger.episode : undefined,
    });
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
    refreshTriggerRef,
    viewKey,
    workspaceGeneration,
    workspaceId,
    initialPage,
  ]);

  useEffect(() => {
    if (loader.pendingPage !== null || !queuedRefreshRef.current) return;
    if (hasTerminalError) {
      queuedRefreshRef.current = false;
      return;
    }
    const timer = setTimeout(() => {
      if (!queuedRefreshRef.current) return;
      if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
      refreshTimerRef.current = null;
      queuedRefreshRef.current = false;
      setRefreshRevision((revision) => revision + 1);
    }, 250);
    return () => clearTimeout(timer);
  }, [loader.pendingPage, queuedRefreshRef, refreshTimerRef, setRefreshRevision, hasTerminalError]);
}

export function useSidebarPageRefresh(options: {
  hasInFlight: () => boolean;
  queued: { current: boolean };
  timer: { current: ReturnType<typeof setTimeout> | null };
  setRefreshRevision: Dispatch<SetStateAction<number>>;
  refreshTriggerRef: { current: SidebarPageRefreshTrigger };
  claimForegroundEpisode: (episode: object) => void;
}) {
  const {
    hasInFlight,
    queued,
    timer,
    setRefreshRevision,
    refreshTriggerRef,
    claimForegroundEpisode,
  } = options;
  return useCallback(
    (kind: "background" | "foreground" = "background") => {
      if (timer.current) clearTimeout(timer.current);
      timer.current = null;
      if (hasInFlight()) {
        queued.current = true;
        // The active shared cycle owns this episode: admitting it again after
        // exhaustion would restart recovery without a later trigger.
        if (kind === "foreground") claimForegroundEpisode({});
        return;
      }
      queued.current = false;
      refreshTriggerRef.current = kind === "foreground" ? { kind, episode: {} } : { kind };
      setRefreshRevision((revision) => revision + 1);
    },
    [hasInFlight, queued, timer, setRefreshRevision, refreshTriggerRef, claimForegroundEpisode],
  );
}
