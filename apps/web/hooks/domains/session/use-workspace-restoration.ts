import { useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { restoreSessionWorkspace } from "@/lib/services/session-recovery-service";
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

export function useWorkspaceRestoration(
  taskId: string | null | undefined,
  sessionId: string | null | undefined,
  explicitEnvironmentId?: string | null,
): WorkspaceRestorationResult {
  const { t } = useTranslation();
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
  const setAgentctlStatus = useAppStore((state) => state.setSessionAgentctlStatus);
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
      await restoreSessionWorkspace(taskId, sessionId, t("task:failedToRestoreWorkspace"));
      if (!callbacks.complete(nextAttempt)) return false;
      setAgentctlStatus?.(sessionId, { status: "ready" });
      bumpWorkspaceFilesRefresh?.(sessionId);
      return true;
    } catch (error) {
      callbacks.fail(nextAttempt, error);
      return false;
    }
  }, [
    bumpWorkspaceFilesRefresh,
    callbacks,
    environmentKey,
    sessionId,
    setAgentctlStatus,
    t,
    taskId,
  ]);

  return {
    status: attempt?.status ?? null,
    attempt,
    restore,
    callbacks,
  };
}
