"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { renameSession } from "@/lib/api/domains/session-api";

/** Mirrors the backend's maxSessionNameLength so the optimistic store update
 * matches what the rename broadcast will echo back. */
export const MAX_SESSION_NAME_LENGTH = 120;

/** Commit a session tab rename: persist via WS and optimistically update the store. */
export function useSessionRenameCommitter(
  sessionId: string | undefined,
  taskId: string | null,
  currentName: string | null,
  onDone: () => void,
) {
  const { t } = useTranslation();
  const appStoreApi = useAppStoreApi();
  const { toast } = useToast();
  return useCallback(
    async (next: string) => {
      onDone();
      if (!sessionId || !taskId) return;
      const normalized = next.trim().slice(0, MAX_SESSION_NAME_LENGTH);
      if ((currentName ?? "") === normalized) return;
      try {
        await renameSession(sessionId, normalized);
        const existing = appStoreApi.getState().taskSessions.items[sessionId];
        if (existing) {
          appStoreApi
            .getState()
            .upsertTaskSessionFromEvent(taskId, { ...existing, name: normalized });
        }
      } catch (error) {
        console.error("rename session:", error);
        toast({
          title: t("task:renameFailed"),
          description: error instanceof Error ? error.message : t("common:unknownError"),
          variant: "error",
        });
      }
    },
    [sessionId, taskId, currentName, appStoreApi, onDone, toast],
  );
}
