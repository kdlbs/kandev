import { ApiError } from "@/lib/api/client";
import {
  listQuickChatSessions,
  restartConfigChat,
  type ConfigChatRestartFailure,
  type StartConfigChatResponse,
  type ListQuickChatSessionsResponse,
} from "@/lib/api/domains/workspace-api";
import { getTaskDeletePreflight } from "@/lib/api/domains/kanban-api";
import { t } from "@/lib/i18n";
import { cleanupTaskStorage } from "@/lib/local-storage";
import type { useAppStoreApi } from "@/components/state-provider";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";

type AppStoreApi = ReturnType<typeof useAppStoreApi>;

const RECONCILE_ATTEMPTS = 60;
const RECONCILE_DELAY_MS = 1000;

function restartFailure(error: unknown): ConfigChatRestartFailure | null {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== "object") return null;
  const body = error.body as Partial<ConfigChatRestartFailure>;
  if (
    typeof body.code !== "string" ||
    typeof body.old_deleted !== "boolean" ||
    !["validate", "stop", "delete", "create", "start"].includes(body.stage ?? "")
  )
    return null;
  return body as ConfigChatRestartFailure;
}

function restartErrorCopy(failure: ConfigChatRestartFailure): string {
  switch (failure.code) {
    case "config_chat_restart_profile_unavailable":
      return t("configChat:restartProfileUnavailable");
    case "config_chat_restart_executor_unavailable":
    case "config_chat_restart_incompatible":
      return t("configChat:restartExecutorUnavailable");
    case "config_chat_restart_stop_failed":
      return t("configChat:restartStopFailed");
    case "config_chat_restart_delete_failed":
      return t("configChat:restartDeleteFailed");
    case "config_chat_restart_create_failed":
      return t("configChat:restartCreateFailed");
    case "config_chat_restart_start_failed":
      return t("configChat:restartStartFailed");
    default:
      return t("configChat:restartNotCompleted");
  }
}

function adoptReplacement(
  store: AppStoreApi,
  workspaceId: string,
  oldSessionId: string,
  response: StartConfigChatResponse,
  profileId?: string,
) {
  clearRetiredSessionCache(store, oldSessionId);
  store.getState().replaceConfigChatSession(workspaceId, oldSessionId, {
    sessionId: response.session_id,
    taskId: response.task_id,
    workspaceId,
    kind: "config",
    agentProfileId: response.agent_profile_id ?? profileId,
  });
}

function clearRetiredSessionCache(store: AppStoreApi, sessionId: string) {
  const state = store.getState();
  const taskId =
    state.quickChat.sessions.find((item) => item.sessionId === sessionId)?.taskId ??
    state.taskSessions.items[sessionId]?.task_id;
  if (!taskId) return;
  const environmentId = state.environmentIdBySessionId[sessionId];
  cleanupTaskStorage(taskId, [sessionId], environmentId ? [environmentId] : []);
  state.removeTaskSession(taskId, sessionId);
}

function removeRetiredSession(store: AppStoreApi, sessionId: string) {
  clearRetiredSessionCache(store, sessionId);
  store.getState().removeQuickChatSession(sessionId);
}

function settleRestart(store: AppStoreApi, workspaceId: string) {
  store.getState().setConfigChatRestart(workspaceId, null);
}

function uncertainRestart(
  store: AppStoreApi,
  workspaceId: string,
  sessionId: string,
  error: string,
): string {
  store
    .getState()
    .setConfigChatRestart(workspaceId, { sessionId, source: "local", status: "uncertain", error });
  return error;
}

export async function reconcileConfigChatRestart(
  store: AppStoreApi,
  workspaceId: string,
  oldSessionId: string,
): Promise<string | null> {
  for (let attempt = 0; attempt < RECONCILE_ATTEMPTS; attempt += 1) {
    try {
      const response = await listQuickChatSessions(workspaceId, {
        cache: "no-store",
        init: { signal: AbortSignal.timeout(10_000) },
      });
      if (response.config_chat_restart_pending) {
        await new Promise((resolve) => setTimeout(resolve, RECONCILE_DELAY_MS));
        continue;
      }
      return settleReconciledRestart(store, workspaceId, oldSessionId, response);
    } catch {
      return uncertainRestart(store, workspaceId, oldSessionId, t("configChat:restartUncertain"));
    }
  }
  return uncertainRestart(store, workspaceId, oldSessionId, t("configChat:restartUncertain"));
}

function settleReconciledRestart(
  store: AppStoreApi,
  workspaceId: string,
  oldSessionId: string,
  response: ListQuickChatSessionsResponse,
): string | null {
  const configSessions = response.sessions.filter(
    (item) => item.workspace_id === workspaceId && item.kind === "config",
  );
  if (configSessions.length > 1)
    return uncertainRestart(store, workspaceId, oldSessionId, t("configChat:restartAmbiguous"));
  for (const row of response.task_sessions) {
    const live = store.getState().taskSessions.items[row.id];
    if (!live || Date.parse(row.updated_at) >= Date.parse(live.updated_at))
      store.getState().setTaskSession(row);
  }
  const session = configSessions[0];
  if (session && session.session_id !== oldSessionId)
    adoptReplacement(store, workspaceId, oldSessionId, session);
  if (!session) removeRetiredSession(store, oldSessionId);
  settleRestart(store, workspaceId);
  if (!session) return t("configChat:restartCreateFailed");
  return session.session_id === oldSessionId ? t("configChat:restartNotCompleted") : null;
}

export async function performConfigChatRestart(
  store: AppStoreApi,
  workspaceId: string,
  session: QuickChatSession,
): Promise<string | null> {
  store.getState().setConfigChatRestart(workspaceId, {
    sessionId: session.sessionId,
    source: "local",
    status: "restarting",
  });
  let submitted = false;
  try {
    const preview = await getTaskDeletePreflight([session.taskId!], false, false);
    if (preview.requires_discard_consent) {
      settleRestart(store, workspaceId);
      return t("configChat:restartDirtyWorktree");
    }
    submitted = true;
    const response = await restartConfigChat(
      workspaceId,
      { task_id: session.taskId!, session_id: session.sessionId },
      preview.confirmation_id,
    );
    adoptReplacement(store, workspaceId, session.sessionId, response, session.agentProfileId);
    settleRestart(store, workspaceId);
    return null;
  } catch (error) {
    if (!submitted) {
      settleRestart(store, workspaceId);
      return t("configChat:restartConfirmationFailed");
    }
    const failure = restartFailure(error);
    if (!failure) return reconcileConfigChatRestart(store, workspaceId, session.sessionId);
    if (failure.old_deleted) {
      if (failure.replacement?.session_id)
        adoptReplacement(
          store,
          workspaceId,
          session.sessionId,
          failure.replacement,
          session.agentProfileId,
        );
      else removeRetiredSession(store, session.sessionId);
    }
    settleRestart(store, workspaceId);
    return restartErrorCopy(failure);
  }
}
