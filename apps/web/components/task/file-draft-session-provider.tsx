"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { useAppStore } from "@/components/state-provider";
import { DiscardLocalChangesDialog } from "@/components/discard-local-changes-dialog";
import { getWebSocketClient } from "@/lib/ws/connection";
import { syncOpenFileFromWorkspace } from "@/hooks/file-editors-sync";
import type { FileChangeNotificationPayload } from "@/lib/types/workspace-files";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { setNavigationBlocker, type NavigationIntent } from "@/lib/routing/navigation-guard";
import { useRegisterMobileNavigationGuard } from "./mobile/mobile-navigation-guard";
import { FileDraftSessionContext } from "./file-draft-session-context";

function useRetainedWorkspaceSync(sessionId: string | null) {
  const connectionStatus = useAppStore((state) => state.connection.status);
  useEffect(() => {
    const client = getWebSocketClient();
    if (!sessionId || !client || connectionStatus !== "connected") return;
    const epoch = useDockviewStore.getState().fileSessionEpoch;
    const refresh = (changes?: FileChangeNotificationPayload["changes"]) => {
      const store = useDockviewStore.getState();
      for (const [fileKey, file] of store.openFiles) {
        if (!changes && !file.originalHash && !file.isDirty) continue;
        if (
          changes &&
          !changes.some(
            (change) =>
              (change.repository_name ?? "") === (file.repo ?? "") &&
              (change.operation === "refresh" || change.path === file.path),
          )
        )
          continue;
        void syncOpenFileFromWorkspace({
          client,
          sessionId,
          fileKey,
          path: file.path,
          repo: file.repo,
          updateFileState: store.updateFileState,
          retainCanonical: true,
          isCurrent: () => useDockviewStore.getState().fileSessionEpoch === epoch,
        });
      }
    };
    const unsubscribe = client.on(
      "session.workspace.file.changes",
      ({ payload }: { payload: FileChangeNotificationPayload }) => {
        if (payload.session_id === sessionId) refresh(payload.changes);
      },
    );
    refresh();
    return unsubscribe;
  }, [connectionStatus, sessionId]);
}

function useDraftNavigationProtection(
  hasDirtyFiles: boolean,
  request: (intent: NavigationIntent) => void,
) {
  useRegisterMobileNavigationGuard(
    useCallback(
      (action) => {
        request({
          proceed: () => {
            void action();
          },
          cancel: () => undefined,
        });
      },
      [request],
    ),
  );
  useEffect(() => {
    if (!hasDirtyFiles) return;
    const unregister = setNavigationBlocker(request);
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => {
      unregister();
      window.removeEventListener("beforeunload", onBeforeUnload);
    };
  }, [hasDirtyFiles, request]);
}

type PendingDraftNavigation = {
  intent: NavigationIntent;
  epoch: symbol;
  instances: Map<string, symbol | undefined>;
};

/** Draft ownership outlives the responsive file surfaces and their panel hosts. */
export function FileDraftSessionProvider({
  children,
  sessionId,
}: {
  children: ReactNode;
  sessionId?: string | null;
}) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId) ?? sessionId ?? null;
  useRetainedWorkspaceSync(activeSessionId);
  const sessionIdRef = useRef(activeSessionId);
  const visitRef = useRef<symbol | null>(null);
  const ownerRef = useRef({ sessionIdRef, visitRef });
  const [pending, setPending] = useState<NavigationIntent | null>(null);
  const pendingRef = useRef<PendingDraftNavigation | null>(null);
  const dirtyFiles = useDockviewStore((state) => state.openFiles);
  const hasDirtyFiles = Array.from(dirtyFiles.values()).some((file) => file.isDirty);
  useLayoutEffect(() => {
    sessionIdRef.current = activeSessionId;
    visitRef.current = Symbol();
    useDockviewStore.getState().activateFileSession(activeSessionId);
    return () => {
      visitRef.current = null;
    };
  }, [activeSessionId]);
  const cancel = useCallback(() => {
    pendingRef.current?.intent.cancel();
    pendingRef.current = null;
    setPending(null);
  }, []);
  const request = useCallback((intent: NavigationIntent) => {
    if (!Array.from(useDockviewStore.getState().openFiles.values()).some((file) => file.isDirty)) {
      intent.proceed();
      return;
    }
    pendingRef.current?.intent.cancel();
    const store = useDockviewStore.getState();
    pendingRef.current = {
      intent,
      epoch: store.fileSessionEpoch,
      instances: new Map(
        Array.from(store.openFiles)
          .filter(([, file]) => file.isDirty)
          .map(([key, file]) => [key, file.instanceId]),
      ),
    };
    setPending(intent);
  }, []);
  useDraftNavigationProtection(hasDirtyFiles, request);
  useEffect(() => cancel, [activeSessionId, cancel]);
  const confirm = useCallback(() => {
    const pending = pendingRef.current;
    pendingRef.current = null;
    setPending(null);
    if (!pending) return;
    const store = useDockviewStore.getState();
    if (
      store.fileSessionEpoch !== pending.epoch ||
      Array.from(pending.instances).some(
        ([key, instance]) => store.openFiles.get(key)?.instanceId !== instance,
      ) ||
      Array.from(store.openFiles).some(([key, file]) => file.isDirty && !pending.instances.has(key))
    ) {
      pending.intent.cancel();
      return;
    }
    for (const key of pending.instances.keys()) store.removeFileState(key);
    pending.intent.proceed();
  }, []);
  return (
    <FileDraftSessionContext.Provider value={ownerRef.current}>
      {children}
      <DiscardLocalChangesDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) cancel();
        }}
        dirtyFiles={Array.from(dirtyFiles.values())
          .filter((file) => file.isDirty)
          .map((file) => file.path)}
        onConfirm={confirm}
        onCancel={cancel}
      />
    </FileDraftSessionContext.Provider>
  );
}
