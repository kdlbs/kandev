"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import type { PRCommitDetail } from "@/lib/types/github";
import type { FileInfo } from "@/lib/state/store";
import type { CommitDetailTarget } from "@/components/task/changes-diff-target";
import { requestCommitDetail } from "@/components/task/commit-detail-request";

export type UseCommitDetailResult = {
  files: Record<string, FileInfo> | null;
  commit: PRCommitDetail | null;
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
};

function targetKey(target: CommitDetailTarget): string {
  if (target.source === "local") return `local:${target.repo ?? ""}:${target.sha}`;
  return `github:${target.workspaceId}:${target.owner}/${target.repo}:${target.sha}`;
}

function buildLocalRequest(
  target: CommitDetailTarget,
  activeSessionId: string | null,
  sessionTaskId: string | undefined,
  activeTaskId: string | null,
  agentctlReady: boolean,
) {
  if (target.source !== "local" || !activeSessionId) return {};
  return {
    local: {
      sessionId: activeSessionId,
      taskId: sessionTaskId ?? activeTaskId ?? null,
      agentctlReady,
    },
  };
}

/**
 * Loads commit details from the source encoded in the target. GitHub targets
 * never use the local session/worktree request, including after an error.
 */
export function useCommitDetail(target: CommitDetailTarget): UseCommitDetailResult {
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
  const { t } = useTranslation();
  const unexpectedError = t("unexpectedResponseFromTheServer", { ns: "github" });
  const requestFailed = t("requestFailed");
  const [files, setFiles] = useState<Record<string, FileInfo> | null>(null);
  const [commit, setCommit] = useState<PRCommitDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestSeqRef = useRef(0);
  const key = targetKey(target);
  const stableTarget = useMemo(() => target, [key]);

  const fetchDetail = useCallback(async () => {
    const requestSeq = ++requestSeqRef.current;
    setLoading(true);
    setError(null);
    try {
      const response = await requestCommitDetail({
        target: stableTarget,
        ...buildLocalRequest(
          stableTarget,
          activeSessionId,
          sessionTaskId,
          activeTaskId,
          agentctlReady,
        ),
      });
      if (requestSeq !== requestSeqRef.current) return;
      setFiles(response?.success && response.files ? response.files : null);
      setCommit(response?.source === "github" ? (response.commit ?? null) : null);
    } catch (err) {
      if (requestSeq !== requestSeqRef.current) return;
      const message = err instanceof Error ? err.message : unexpectedError;
      setFiles(null);
      setCommit(null);
      setError(message);
      toast({
        title: requestFailed,
        description: message,
        variant: "error",
      });
    } finally {
      if (requestSeq === requestSeqRef.current) setLoading(false);
    }
  }, [
    activeSessionId,
    activeTaskId,
    agentctlReady,
    requestFailed,
    sessionTaskId,
    stableTarget,
    toast,
    unexpectedError,
  ]);

  useEffect(() => {
    void fetchDetail();
  }, [fetchDetail]);

  return { files, commit, loading, error, refetch: fetchDetail };
}
