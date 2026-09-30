"use client";

import { useEditors } from "@/hooks/domains/settings/use-editors";
import { create } from "zustand";
import { openSessionFolder } from "@/lib/api";
import { useRequest } from "@/lib/http/use-request";
import { useToast } from "@/components/toast-provider";
import { t } from "@/lib/i18n";

// Native launches are shared by every folder control for a session.
const usePendingFolders = create<{ sessions: ReadonlySet<string> }>(() => ({
  sessions: new Set(),
}));

function setFolderPending(sessionId: string, pending: boolean) {
  usePendingFolders.setState((state) => {
    const sessions = new Set(state.sessions);
    if (pending) sessions.add(sessionId);
    else sessions.delete(sessionId);
    return { sessions };
  });
}

export function useOpenSessionFolder(sessionId?: string | null) {
  const { folderOpeningAvailable } = useEditors();
  const { toast } = useToast();
  const isLoading = usePendingFolders((state) =>
    Boolean(sessionId && state.sessions.has(sessionId)),
  );
  const request = useRequest(async (worktreeId?: string) => {
    if (!sessionId) return null;
    return (
      (await openSessionFolder(
        sessionId,
        { cache: "no-store" },
        worktreeId ? { worktree_id: worktreeId } : undefined,
      )) ?? null
    );
  });

  return {
    open: async (worktreeId?: string) => {
      if (
        !sessionId ||
        !folderOpeningAvailable ||
        usePendingFolders.getState().sessions.has(sessionId)
      )
        return null;
      setFolderPending(sessionId, true);
      try {
        return await request.run(worktreeId);
      } catch {
        toast({ title: t("editors:failedToOpenFolder"), variant: "error" });
        return null;
      } finally {
        setFolderPending(sessionId, false);
      }
    },
    available: folderOpeningAvailable,
    status: isLoading ? "loading" : request.status,
    isLoading,
  };
}
