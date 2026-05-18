"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { requestCommitDiff } from "@/components/task/commit-diff-request";
import type { FileInfo } from "@/lib/state/store";

type UseCommitDiffResult = {
  files: Record<string, FileInfo> | null;
  loading: boolean;
  refetch: () => Promise<void>;
};

/**
 * Fetches a commit's per-file diff via WebSocket and re-fetches once
 * agentctl transitions from not-ready to ready. Used by both desktop
 * (CommitDetailPanel) and mobile (CommitDiffView in the diff sheet).
 */
export function useCommitDiff(commitSha: string, repo?: string): UseCommitDiffResult {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const sessionTaskId = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId]?.task_id : undefined,
  );
  const agentctlReady = useAppStore((state) =>
    activeSessionId
      ? state.sessionAgentctl.itemsBySessionId[activeSessionId]?.status === "ready"
      : false,
  );
  const { toast } = useToast();

  const [files, setFiles] = useState<Record<string, FileInfo> | null>(null);
  const [loading, setLoading] = useState(false);
  const requestSeqRef = useRef(0);

  const fetchDiff = useCallback(async () => {
    const requestSeq = ++requestSeqRef.current;
    if (!activeSessionId) {
      if (requestSeq === requestSeqRef.current) {
        setFiles(null);
        setLoading(false);
      }
      return;
    }
    setLoading(true);
    try {
      const response = await requestCommitDiff({
        sessionId: activeSessionId,
        taskId: sessionTaskId ?? activeTaskId ?? null,
        commitSha,
        agentctlReady,
        repo,
      });
      if (requestSeq !== requestSeqRef.current) return;
      setFiles(response?.success && response.files ? response.files : null);
    } catch (err) {
      if (requestSeq !== requestSeqRef.current) return;
      toast({
        title: "Failed to load commit diff",
        description: err instanceof Error ? err.message : "An unexpected error occurred",
        variant: "error",
      });
    } finally {
      if (requestSeq === requestSeqRef.current) {
        setLoading(false);
      }
    }
  }, [activeSessionId, activeTaskId, agentctlReady, commitSha, repo, sessionTaskId, toast]);

  useEffect(() => {
    fetchDiff();
  }, [fetchDiff]);

  return { files, loading, refetch: fetchDiff };
}
