import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";
import type { TaskEventPayload } from "@/lib/types/backend";

// i18n-exempt: wire value. The backend sends this exact title for an untitled
// quick chat and the check below compares it with `!==` to tell a real,
// user-chosen title apart from the placeholder.
const QUICK_CHAT_PLACEHOLDER_TITLE = "Quick Chat";

function readMetadataString(
  metadata: Record<string, unknown> | null | undefined,
  key: string,
): string | undefined {
  const value = metadata?.[key];
  return typeof value === "string" && value ? value : undefined;
}

/**
 * Maps a task lifecycle event onto a quick-chat tab, or null when the task does
 * not back one.
 *
 * Mirrors the backend's restorable-quick-chat rules (see
 * `IsRestorableQuickChatTask`): ephemeral, not workflow-bound, not an
 * automation run. A task with no primary session yet is skipped — the tab is
 * keyed by session id, and a later event carries it once the agent launches.
 */
export function quickChatSessionFromTaskEvent(payload: TaskEventPayload): QuickChatSession | null {
  if (!payload.is_ephemeral) return null;
  if (payload.workflow_id) return null;
  if (payload.origin === "automation_run") return null;
  const sessionId = payload.primary_session_id;
  const workspaceId = payload.workspace_id;
  if (!sessionId || !workspaceId) return null;

  const configMode = payload.metadata?.config_mode === true;
  return {
    kind: configMode ? "config" : "chat",
    sessionId,
    workspaceId,
    taskId: payload.task_id,
    name:
      payload.title && payload.title !== QUICK_CHAT_PLACEHOLDER_TITLE ? payload.title : undefined,
    agentProfileId: readMetadataString(payload.metadata, "agent_profile_id"),
  };
}

/**
 * Reflects a quick chat started (or renamed) on another device into this
 * client's tab strip. Ephemeral tasks are skipped by the kanban handlers, so
 * without this a quick chat only ever existed on the device that created it.
 */
export function syncQuickChatFromTaskEvent(
  store: StoreApi<AppState>,
  payload: TaskEventPayload,
): void {
  // An archived quick chat is gone from the tab strip just like a deleted one.
  if (payload.archived_at) {
    store.getState().removeQuickChatSessionsForTask(payload.task_id);
    return;
  }
  const session = quickChatSessionFromTaskEvent(payload);
  if (!session) return;
  store.getState().upsertQuickChatSessionFromEvent(session);
}
