import { useCallback, useLayoutEffect, useRef, useSyncExternalStore } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { treePath } from "@/hooks/file-rename-continuity";
import type { OpenFileTab } from "@/lib/types/backend";
import {
  captureTabletFileOpen,
  hasPendingTabletFileOpen,
  subscribeTabletFileOpen,
  type TabletFileOpen,
} from "@/lib/state/tablet-file-authority";

export function useTabletFileOpen(sessionId: string | null) {
  const store = useAppStoreApi();
  const visit = useRef<object>({});
  const pending = useRef<TabletFileOpen | null>(null);
  useLayoutEffect(() => {
    visit.current = {};
    return () => {
      visit.current = {};
      pending.current?.finish();
    };
  }, [store, sessionId]);
  return useCallback(
    (file: Pick<OpenFileTab, "path" | "repo">) => {
      if (!sessionId) return null;
      const owner = visit.current;
      const operation = captureTabletFileOpen(
        store,
        sessionId,
        treePath(file) ?? file.path,
        () => visit.current === owner,
      );
      pending.current = operation;
      return operation;
    },
    [store, sessionId],
  );
}

export function usePendingTabletFileOpen(sessionId: string | null): boolean {
  const store = useAppStoreApi();
  const subscribe = useCallback(
    (listener: () => void) => {
      const releaseOpen = subscribeTabletFileOpen(store, listener);
      const releaseStore = store.subscribe(listener);
      return () => {
        releaseOpen();
        releaseStore();
      };
    },
    [store],
  );
  const snapshot = useCallback(
    () => (sessionId ? hasPendingTabletFileOpen(store, sessionId) : false),
    [store, sessionId],
  );
  return useSyncExternalStore(subscribe, snapshot, snapshot);
}
