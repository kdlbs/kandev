"use client";

import { useCallback, useMemo } from "react";
import { useOptionalAppStore } from "@/components/state-provider";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

/**
 * Props forwarded to every plugin component registered for the `chat-top-bar`
 * slot (`registry.registerComponent("chat-top-bar", Component)`). This is the
 * session top bar's right-hand cluster, beside the CPU/DB metrics and the
 * document / editors / debug controls — the place for at-a-glance status a
 * plugin wants to surface for the current task.
 *
 * A task can hold several sessions; the top bar is bound to one at a time
 * (`activeSessionId`), so both it and the full `sessionIds` list are provided,
 * mirroring the `chat-input-actions` slot. These are kandev session ids;
 * resolving one to an agent/ACP transcript id (e.g. to key cost data on a
 * session) is the plugin's job — do it server-side in the plugin backend
 * through the Host data API, not here. See PLUGIN-API.md.
 */
export type ChatTopBarSlotProps = {
  /** Task the top bar belongs to, or null before one exists. */
  taskId: string | null;
  /** Display title of the task, when known. */
  taskTitle?: string;
  /** Workspace the task lives in, when known. */
  workspaceId: string | null;
  /** Session the top bar is currently bound to, or null before one exists. */
  activeSessionId: string | null;
  /** Every kandev session id on the task (includes `activeSessionId`). */
  sessionIds: string[];
};

const EMPTY_SESSIONS: TaskSession[] = [];

/**
 * Plugin extension point in the session top bar, rendered alongside the
 * first-party controls (CPU/DB metrics, document/editor menus, debug toggle).
 * Renders every plugin component registered for the `chat-top-bar` slot (each
 * isolated behind its own error boundary via `PluginSlot`) and forwards the
 * current task, workspace, and all of its session ids as `slotProps`.
 */
export function TaskTopBarPluginActions(props: {
  sessionId: string | null;
  taskId: string | null;
  taskTitle?: string;
  workspaceId: string | null;
}) {
  const { sessionId, taskId, taskTitle, workspaceId } = props;
  // itemsByTaskId holds a stable per-task array reference (updated only when
  // that task's sessions change), so selecting it avoids a new-array-per-render.
  // Read optionally so the top bar can render in isolation (unit tests) without
  // a StateProvider.
  const selectSessions = useCallback(
    (s: AppState): TaskSession[] =>
      taskId ? (s.taskSessionsByTask.itemsByTaskId[taskId] ?? EMPTY_SESSIONS) : EMPTY_SESSIONS,
    [taskId],
  );
  const taskSessions = useOptionalAppStore(selectSessions, EMPTY_SESSIONS);

  const slotProps = useMemo<ChatTopBarSlotProps>(() => {
    const sessionIds: string[] = taskSessions.map((session) => session.id);
    // The active session may not yet be in the store list (freshly prepared);
    // make sure the plugin always receives it.
    if (sessionId && !sessionIds.includes(sessionId)) sessionIds.unshift(sessionId);
    return { taskId, taskTitle, workspaceId, activeSessionId: sessionId, sessionIds };
  }, [taskSessions, sessionId, taskId, taskTitle, workspaceId]);

  return <PluginSlot name="chat-top-bar" slotProps={slotProps} />;
}
