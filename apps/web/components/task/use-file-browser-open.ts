import { useCallback, useLayoutEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { isRenameReserved, trackRenameOpen } from "@/hooks/file-rename-continuity";
import type { OpenFileTab } from "@/lib/types/backend";
import { fetchAndOpenFile } from "./file-browser-actions";
import { useTabletFileOpen } from "@/hooks/domains/session/use-tablet-file-admission";
import { bindTabletFileOpen } from "@/lib/state/tablet-file-authority";

export function useFileBrowserOpen(sessionId: string, onOpenFile: (file: OpenFileTab) => void) {
  const { toast } = useToast();
  const { t } = useTranslation();
  const store = useAppStoreApi();
  const openFileAbortRef = useRef<AbortController | null>(null);
  const beginOpen = useTabletFileOpen(sessionId);
  useLayoutEffect(
    () => () => {
      openFileAbortRef.current?.abort();
      openFileAbortRef.current = null;
    },
    [store, sessionId],
  );
  return useCallback(
    (path: string) => {
      if (isRenameReserved(store, sessionId, path)) {
        toast({ title: t("task:failedToOpenFile"), variant: "error" });
        return;
      }
      openFileAbortRef.current?.abort();
      const pendingOpen = trackRenameOpen(store, sessionId, path);
      const controller = new AbortController();
      openFileAbortRef.current = controller;
      const operation = beginOpen({ path });
      if (!operation) return;
      let delivered = false;
      return fetchAndOpenFile(
        sessionId,
        path,
        (file) => {
          if (
            operation.current(file) &&
            pendingOpen.current() &&
            !isRenameReserved(store, sessionId, path)
          ) {
            bindTabletFileOpen(file, operation);
            onOpenFile(file);
            delivered = true;
          }
        },
        toast,
        { signal: controller.signal },
      ).finally(() => {
        pendingOpen.dispose();
        if (!delivered) operation.finish();
        if (openFileAbortRef.current === controller) openFileAbortRef.current = null;
      });
    },
    [sessionId, onOpenFile, toast, store, t, beginOpen],
  );
}
