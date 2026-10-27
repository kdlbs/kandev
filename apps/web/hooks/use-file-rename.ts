import { useCallback, useLayoutEffect, useRef, type RefObject } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useTranslation } from "react-i18next";
import { getWebSocketClient } from "@/lib/ws/connection";
import { renameFile } from "@/lib/ws/workspace-files";
import { isWorkspaceTreePath } from "@/lib/workspace-file-path";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import {
  captureDockviewRename,
  renameScope,
  reserveRename,
  type RenameAdapter,
} from "./file-rename-continuity";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { runFileOp } from "./file-operation-utils";

function validPaths(oldPath: string, newPath: string) {
  return !!oldPath && !!newPath && isWorkspaceTreePath(oldPath) && isWorkspaceTreePath(newPath);
}

function isOwnerCurrent(args: {
  store: StoreApi<AppState>;
  session: string;
  selected: string;
  scope: string;
  owner: symbol;
  visit: RefObject<symbol | null>;
}) {
  const { store, session, selected, scope, owner, visit } = args;
  return (
    visit.current === owner &&
    store.getState().tasks.activeSessionId === selected &&
    renameScope(store, selected) === scope &&
    renameScope(store, session) === scope
  );
}

function captureRename(
  options: { dockview?: boolean; adapter?: RefObject<RenameAdapter | null> } | undefined,
  oldPath: string,
  newPath: string,
  selectedSession: string,
  workspaceRoot?: string,
) {
  if (options?.dockview === false)
    return (
      options.adapter?.current?.capture(oldPath, newPath) ?? {
        collision: false,
        publish: () => true,
      }
    );
  return captureDockviewRename(oldPath, newPath, selectedSession, workspaceRoot);
}

const ERROR_VARIANT = "error" as const;
export function useFileRename(
  sessionId: string | null,
  options?: { dockview?: boolean; adapter?: RefObject<RenameAdapter | null> },
) {
  const { toast } = useToast();
  const { t } = useTranslation("task");
  const unknownError = t("common:anUnknownErrorOccurred");
  const store = useAppStoreApi();
  const selectedSession = useAppStore((state) => state.tasks.activeSessionId);
  const visit = useRef<symbol | null>(null);
  useLayoutEffect(() => {
    const token = Symbol();
    visit.current = token;
    return () => {
      if (visit.current === token) visit.current = null;
    };
  }, [selectedSession]);
  const handleRenameFile = useCallback(
    async (oldPath: string, newPath: string): Promise<boolean> => {
      const client = getWebSocketClient();
      if (!client || !sessionId) return false;
      if (!selectedSession || !visit.current || !validPaths(oldPath, newPath)) return false;
      const scope = renameScope(store, selectedSession);
      if (scope !== renameScope(store, sessionId)) return false;
      const owner = visit.current;
      const workspaceRoot = getSessionWorkspacePath(
        store.getState().taskSessions.items[selectedSession],
      );
      const captured = captureRename(options, oldPath, newPath, selectedSession, workspaceRoot);
      const release = captured.collision ? null : reserveRename(store, scope, oldPath, newPath);
      if (!release) {
        toast({
          title: t("task:failedToRenameItem"),
          variant: ERROR_VARIANT,
        });
        return false;
      }
      try {
        const ok = await runFileOp(
          () => renameFile(client, sessionId, oldPath, newPath),
          t("task:failedToRenameItem"),
          unknownError,
          toast,
        );
        if (
          ok &&
          isOwnerCurrent({
            store,
            session: sessionId,
            selected: selectedSession,
            scope,
            owner,
            visit,
          })
        ) {
          if (!captured.publish())
            toast({ title: t("task:failedToOpenFile"), variant: ERROR_VARIANT });
        }
        return ok;
      } finally {
        release();
      }
    },
    [
      sessionId,
      selectedSession,
      store,
      t,
      toast,
      unknownError,
      options?.dockview,
      options?.adapter,
    ],
  );

  return handleRenameFile;
}
