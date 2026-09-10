import { useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { restoreSessionWorkspace } from "@/lib/services/session-recovery-service";
import type { SessionAgentctlStatus } from "@/lib/state/slices/session/types";
import {
  resolveWorkspaceRestorationKey,
  sanitizeWorkspaceRestorationDetails,
  type WorkspaceRestorationAttempt,
  type WorkspaceRestorationStatus,
} from "@/lib/state/slices/session-runtime/workspace-restoration";

export type WorkspaceRestorationCallbacks = {
  begin: (taskId: string, sessionId: string) => WorkspaceRestorationAttempt | null;
  complete: (attempt: WorkspaceRestorationAttempt) => boolean;
  fail: (attempt: WorkspaceRestorationAttempt, error: unknown) => boolean;
  clear: (attempt: WorkspaceRestorationAttempt) => boolean;
};

export type WorkspaceRestorationResult = {
  status: WorkspaceRestorationStatus | null;
  attempt: WorkspaceRestorationAttempt | null;
  restore: () => Promise<boolean>;
  callbacks: WorkspaceRestorationCallbacks | null;
};

type WorkspaceRestoreResponse = Awaited<ReturnType<typeof restoreSessionWorkspace>>;

function settleWorkspaceRestoreResponse({
  response,
  agentctlStatus,
  attempt,
  callbacks,
  sessionId,
  t,
  bumpWorkspaceFilesRefresh,
}: {
  response: WorkspaceRestoreResponse;
  agentctlStatus: SessionAgentctlStatus | undefined;
  attempt: WorkspaceRestorationAttempt;
  callbacks: WorkspaceRestorationCallbacks;
  sessionId: string;
  t: (key: string) => string;
  bumpWorkspaceFilesRefresh?: (sessionId: string) => void;
}): "pending" | "ready" | "failed" {
  const responseExecutionId = response.agent_execution_id?.trim();
  if (!responseExecutionId || agentctlStatus?.agentExecutionId !== responseExecutionId) {
    return "pending";
  }
  if (agentctlStatus.status === "error") {
    callbacks.fail(attempt, agentctlStatus.errorMessage || t("task:failedToRestoreWorkspace"));
    return "failed";
  }
  if (agentctlStatus.status !== "ready") return "pending";
  if (!callbacks.complete(attempt)) return "failed";
  bumpWorkspaceFilesRefresh?.(sessionId);
  return "ready";
}

function isCurrentWorkspaceAttempt(
  storeApi: ReturnType<typeof useAppStoreApi>,
  environmentKey: string,
  attempt: WorkspaceRestorationAttempt,
): boolean {
  const currentAttempt = storeApi.getState().workspaceRestoration.byEnvironmentId[environmentKey];
  return Boolean(
    currentAttempt &&
    currentAttempt.revision === attempt.revision &&
    currentAttempt.taskId === attempt.taskId &&
    currentAttempt.sessionId === attempt.sessionId,
  );
}

export function useWorkspaceRestoration(
  taskId: string | null | undefined,
  sessionId: string | null | undefined,
  explicitEnvironmentId?: string | null,
): WorkspaceRestorationResult {
  const { t } = useTranslation();
  const storeApi = useAppStoreApi();
  const sessionEnvironmentId = useAppStore((state) =>
    sessionId
      ? (state.environmentIdBySessionId?.[sessionId] ??
        state.taskSessions?.items?.[sessionId]?.task_environment_id ??
        null)
      : null,
  );
  const environmentId = explicitEnvironmentId ?? sessionEnvironmentId;
  const environmentKey = resolveWorkspaceRestorationKey(sessionId, environmentId);
  const attempt = useAppStore((state) =>
    environmentKey ? (state.workspaceRestoration?.byEnvironmentId?.[environmentKey] ?? null) : null,
  );
  const beginAction = useAppStore((state) => state.beginWorkspaceRestoration);
  const completeAction = useAppStore((state) => state.completeWorkspaceRestoration);
  const failAction = useAppStore((state) => state.failWorkspaceRestoration);
  const clearAction = useAppStore((state) => state.clearWorkspaceRestoration);
  const bumpWorkspaceFilesRefresh = useAppStore((state) => state.bumpWorkspaceFilesRefresh);

  const callbacks = useMemo<WorkspaceRestorationCallbacks | null>(
    () =>
      beginAction
        ? {
            begin: (nextTaskId, nextSessionId) => {
              if (!environmentKey) return null;
              return beginAction(nextTaskId, nextSessionId, environmentKey);
            },
            complete: (nextAttempt) => completeAction?.(nextAttempt) ?? false,
            fail: (nextAttempt, error) =>
              failAction?.(nextAttempt, sanitizeWorkspaceRestorationDetails(error)) ?? false,
            clear: (nextAttempt) => clearAction?.(nextAttempt) ?? false,
          }
        : null,
    [beginAction, clearAction, completeAction, environmentKey, failAction],
  );

  const restore = useCallback(async (): Promise<boolean> => {
    if (!taskId || !sessionId || !environmentKey || !callbacks) return false;
    const nextAttempt = callbacks.begin(taskId, sessionId);
    if (!nextAttempt) return false;
    try {
      const response = await restoreSessionWorkspace(
        taskId,
        sessionId,
        t("task:failedToRestoreWorkspace"),
      );
      const agentctlStatus = storeApi.getState().sessionAgentctl.itemsBySessionId[sessionId];
      if (
        settleWorkspaceRestoreResponse({
          response,
          agentctlStatus,
          attempt: nextAttempt,
          callbacks,
          sessionId,
          t,
          bumpWorkspaceFilesRefresh,
        }) === "failed"
      ) {
        return false;
      }
      if (!isCurrentWorkspaceAttempt(storeApi, environmentKey, nextAttempt)) return false;
      // The response admits the workspace. A matching agentctl_ready event
      // settles the attempt when readiness is still in flight.
      return true;
    } catch (error) {
      callbacks.fail(nextAttempt, error);
      return false;
    }
  }, [bumpWorkspaceFilesRefresh, callbacks, environmentKey, sessionId, storeApi, t, taskId]);

  return {
    status: attempt?.status ?? null,
    attempt,
    restore,
    callbacks,
  };
}
