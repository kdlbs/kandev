import { useCallback, useEffect, useMemo, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { useTaskLsp } from "@/hooks/domains/lsp/use-task-lsp";
import { lspClientManager, toLspLanguage, type LspStatus } from "@/lib/lsp/lsp-client-manager";
import { EMPTY_LSP_PROGRESS, type LspProgressSnapshot } from "@/lib/lsp/lsp-progress";
import type { TaskLspLanguageSnapshot } from "@/lib/types/http-lsp";
import { t } from "@/lib/i18n";

const DISABLED: LspStatus = { state: "disabled" };
const PHASE_STATUS: Partial<Record<TaskLspLanguageSnapshot["phase"], LspStatus>> = {
  off: DISABLED,
  waiting_for_task: DISABLED,
  queued: { state: "starting" },
  installing: { state: "installing" },
  starting: { state: "starting" },
  process_started: { state: "starting" },
  initializing: { state: "starting" },
  ready: { state: "ready" },
  stopping: { state: "stopping" },
};

function managerKey(taskId: string | null, language: string | null): string | null {
  return taskId && language ? `${taskId}:${language}` : null;
}

function taskStatus(snapshot: TaskLspLanguageSnapshot | undefined): LspStatus {
  if (!snapshot) return DISABLED;
  if (snapshot.phase === "unsupported") {
    return {
      state: "unavailable",
      cause: "unsupported_executor",
      reason: snapshot.error_message ?? snapshot.error_code ?? t("lsp:taskExecutorUnsupported"),
    };
  }
  if (snapshot.phase === "error") {
    return {
      state: "error",
      reason: snapshot.error_message ?? snapshot.error_code ?? t("lsp:connectionClosed"),
    };
  }
  return PHASE_STATUS[snapshot.phase] ?? DISABLED;
}

function taskProgress(snapshot: TaskLspLanguageSnapshot | undefined): LspProgressSnapshot {
  if (!snapshot) return EMPTY_LSP_PROGRESS;
  return {
    initializingSince: snapshot.initialize_started_at
      ? Date.parse(snapshot.initialize_started_at)
      : null,
    active: snapshot.progress.map((item) => ({
      token: item.token,
      title: item.title,
      message: item.message ?? null,
      percentage: item.percentage ?? null,
      startedAt: Date.parse(item.started_at),
    })),
    completed: null,
    hasReportedProgress: snapshot.progress.length > 0,
  };
}

function useTaskIdForSession(sessionId: string | null): string | null {
  return useAppStore((state) => {
    if (!sessionId) return state.tasks.activeTaskId;
    return state.taskSessions.items[sessionId]?.task_id ?? null;
  });
}

export function useLspStatus(sessionId: string | null, lspLanguage: string | null) {
  const taskId = useTaskIdForSession(sessionId);
  const taskLsp = useTaskLsp(taskId);
  const snapshot = lspLanguage ? taskLsp.byLanguage[lspLanguage] : undefined;
  const key = managerKey(taskId, lspLanguage);
  const attachmentStatus = useSyncExternalStore(
    (callback) => {
      if (!key) return () => {};
      return lspClientManager.onChange((changedKey) => {
        if (changedKey === key) callback();
      });
    },
    () => (taskId && lspLanguage ? lspClientManager.getStatus(taskId, lspLanguage) : DISABLED),
  );
  const lifecycleStatus = taskStatus(snapshot);
  const status =
    lifecycleStatus.state === "ready" && sessionId && attachmentStatus.state !== "disabled"
      ? attachmentStatus
      : lifecycleStatus;
  const progress = useMemo(() => taskProgress(snapshot), [snapshot]);
  const toggle = useCallback(() => {
    if (!lspLanguage) return;
    if (
      lifecycleStatus.state === "disabled" ||
      lifecycleStatus.state === "error" ||
      lifecycleStatus.state === "unavailable"
    ) {
      void taskLsp.start(lspLanguage).catch(() => undefined);
      return;
    }
    if (lifecycleStatus.state !== "stopping") {
      void taskLsp.stop(lspLanguage).catch(() => undefined);
    }
  }, [lifecycleStatus.state, lspLanguage, taskLsp]);
  return { status, progress, toggle, taskId, snapshot, attachmentStatus };
}

export function useLsp(
  sessionId: string | null,
  monacoLanguage: string,
): {
  status: LspStatus;
  progress: LspProgressSnapshot;
  lspLanguage: string | null;
  taskId: string | null;
  toggle: () => void;
} {
  const lspLanguage = toLspLanguage(monacoLanguage);
  const { status, progress, toggle, taskId, snapshot } = useLspStatus(sessionId, lspLanguage);
  useEffect(() => {
    if (!taskId || !sessionId || !lspLanguage || snapshot?.phase !== "ready") return;
    return lspClientManager.connect(taskId, sessionId, lspLanguage);
  }, [taskId, sessionId, lspLanguage, snapshot?.generation, snapshot?.phase]);

  return { status, progress, lspLanguage, taskId, toggle };
}
