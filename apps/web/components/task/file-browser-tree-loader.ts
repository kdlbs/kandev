import { useCallback, useRef } from "react";
import type React from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { FileTreeNode } from "@/lib/types/backend";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import {
  completeRestoredTree,
  fetchRestoredTree,
  removeFailedExpansions,
} from "./file-browser-restore";
import type { LoadState } from "./file-browser-hooks";

const debugLoad = createDebugLogger("file-browser:load");
const MAX_RETRY_ATTEMPTS = 4;
const RETRY_DELAYS_MS = [1000, 2000, 5000, 10000];

export type TreeLoadOwner = { sessionId: string; resetKey: string; generation: number };
export type TreeLoaderContext = {
  sessionId: string;
  effectiveResetKey: string;
  clearRetryTimer: () => void;
  retryAttemptRef: React.MutableRefObject<number>;
  retryTimerRef: React.MutableRefObject<NodeJS.Timeout | null>;
  restoreExpandedPathsRef: React.MutableRefObject<string[]>;
  hasInitializedExpandedRef: React.MutableRefObject<string | null>;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  setExpandedPaths: React.Dispatch<React.SetStateAction<Set<string>>>;
  setIsLoadingTree: React.Dispatch<React.SetStateAction<boolean>>;
  setLoadState: React.Dispatch<React.SetStateAction<LoadState>>;
  setLoadError: React.Dispatch<React.SetStateAction<string | null>>;
};

function logLoad(event: string, data: Record<string, unknown>) {
  if (isDebug()) debugLoad(event, data);
}

function useTreeLoadOwner(sessionId: string, resetKey: string) {
  const ownerRef = useRef<TreeLoadOwner>({ sessionId, resetKey, generation: 0 });
  if (ownerRef.current.sessionId !== sessionId || ownerRef.current.resetKey !== resetKey) {
    ownerRef.current = { sessionId, resetKey, generation: ownerRef.current.generation + 1 };
  }
  return ownerRef;
}

function requireWebSocketClient() {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket client not available");
  return client;
}

function scheduleRetry({
  error,
  isCurrentLoad,
  owner,
  retryAttemptRef,
  retryTimerRef,
  clearRetryTimer,
  setLoadError,
  setLoadState,
  retry,
}: {
  error: unknown;
  isCurrentLoad: () => boolean;
  owner: TreeLoadOwner;
  retryAttemptRef: React.MutableRefObject<number>;
  retryTimerRef: React.MutableRefObject<NodeJS.Timeout | null>;
  clearRetryTimer: () => void;
  setLoadError: React.Dispatch<React.SetStateAction<string | null>>;
  setLoadState: React.Dispatch<React.SetStateAction<LoadState>>;
  retry: () => void;
}) {
  if (!isCurrentLoad()) return;
  const message = error instanceof Error ? error.message : "Failed to load file tree";
  setLoadError(message);
  if (retryAttemptRef.current >= MAX_RETRY_ATTEMPTS) {
    setLoadState("manual");
    logLoad("gave-up", { sessionId: owner.sessionId, error: message });
    return;
  }
  const delay = RETRY_DELAYS_MS[Math.min(retryAttemptRef.current, RETRY_DELAYS_MS.length - 1)];
  retryAttemptRef.current += 1;
  setLoadState("waiting");
  clearRetryTimer();
  logLoad("retry", {
    sessionId: owner.sessionId,
    attempt: retryAttemptRef.current,
    delayMs: delay,
    error: message,
  });
  retryTimerRef.current = setTimeout(retry, delay);
}

async function restoreTree({
  owner,
  paths,
  isCurrentLoad,
  setExpandedPaths,
}: {
  owner: TreeLoadOwner;
  paths: string[];
  isCurrentLoad: () => boolean;
  setExpandedPaths: React.Dispatch<React.SetStateAction<Set<string>>>;
}) {
  const hydrated = await fetchRestoredTree({
    client: requireWebSocketClient(),
    owner,
    paths,
    isCurrentLoad,
  });
  return completeRestoredTree(hydrated, isCurrentLoad, (failedPaths) =>
    setExpandedPaths((previous) => removeFailedExpansions(Array.from(previous), failedPaths)),
  );
}

export function useTreeLoader(ctx: TreeLoaderContext) {
  const {
    clearRetryTimer,
    retryAttemptRef,
    retryTimerRef,
    restoreExpandedPathsRef,
    hasInitializedExpandedRef,
    setTree,
    setExpandedPaths,
    setIsLoadingTree,
    setLoadState,
    setLoadError,
  } = ctx;
  const ownerRef = useTreeLoadOwner(ctx.sessionId, ctx.effectiveResetKey);
  const loadInFlightGenerationRef = useRef<number | null>(null);
  const loadRequestRef = useRef(0);
  const loadTree = useCallback(
    async (options?: { resetRetry?: boolean; restoreExpandedPaths?: string[] }) => {
      const owner = ownerRef.current;
      if (loadInFlightGenerationRef.current === owner.generation) {
        logLoad("skip-in-flight", { sessionId: owner.sessionId, resetKey: owner.resetKey });
        return;
      }
      const requestId = ++loadRequestRef.current;
      const isCurrentLoad = () =>
        loadRequestRef.current === requestId && ownerRef.current.generation === owner.generation;
      const restorePaths = options?.restoreExpandedPaths ?? restoreExpandedPathsRef.current;
      loadInFlightGenerationRef.current = owner.generation;
      setIsLoadingTree(true);
      setLoadState("loading");
      setLoadError(null);
      if (options?.resetRetry) {
        retryAttemptRef.current = 0;
        clearRetryTimer();
      }
      logLoad("start", {
        sessionId: owner.sessionId,
        resetKey: owner.resetKey,
        resetRetry: options?.resetRetry === true,
        retryAttempt: retryAttemptRef.current,
      });
      try {
        const completed = await restoreTree({
          owner,
          paths: restorePaths,
          isCurrentLoad,
          setExpandedPaths,
        });
        if (!completed) return;
        hasInitializedExpandedRef.current = owner.resetKey;
        setTree(completed.tree);
        setLoadState("loaded");
        retryAttemptRef.current = 0;
        clearRetryTimer();
        logLoad("loaded", {
          sessionId: owner.sessionId,
          rootPath: completed.root?.path ?? null,
          children: completed.root?.children?.length ?? 0,
        });
      } catch (error) {
        scheduleRetry({
          error,
          isCurrentLoad,
          owner,
          retryAttemptRef,
          retryTimerRef,
          clearRetryTimer,
          setLoadError,
          setLoadState,
          retry: () => {
            retryTimerRef.current = null;
            if (!isCurrentLoad()) return;
            void loadTree({ restoreExpandedPaths: restorePaths });
          },
        });
      } finally {
        if (isCurrentLoad()) {
          setIsLoadingTree(false);
          loadInFlightGenerationRef.current = null;
        }
      }
    },
    [
      clearRetryTimer,
      retryAttemptRef,
      retryTimerRef,
      restoreExpandedPathsRef,
      hasInitializedExpandedRef,
      setTree,
      setExpandedPaths,
      setIsLoadingTree,
      setLoadError,
      setLoadState,
    ],
  );
  return loadTree;
}
