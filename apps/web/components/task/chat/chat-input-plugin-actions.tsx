"use client";

import { useCallback, useMemo } from "react";
import { useOptionalAppStore } from "@/components/state-provider";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

/**
 * Props forwarded to every plugin component registered for the
 * `chat-input-actions` slot (`registry.registerComponent("chat-input-actions",
 * Component)`). A task can hold several sessions; the composer targets one at a
 * time (`activeSessionId`), so both it and the full `sessionIds` list are
 * provided.
 *
 * These are kandev session ids. Resolving them to an agent/ACP transcript id
 * (e.g. to key tokscale cost data on a session) is the plugin's job — do it
 * server-side in the plugin backend through the Host data API, not here. The
 * host only propagates the ids. See PLUGIN-API.md.
 */
export type ChatInputActionsSlotProps = {
  /** Task the composer belongs to, or null for task-less quick chat. */
  taskId: string | null;
  /** Display title of the task, when known. */
  taskTitle?: string;
  /** Session the composer is currently bound to, or null before one exists. */
  activeSessionId: string | null;
  /** Every kandev session id on the task (includes `activeSessionId`). */
  sessionIds: string[];
};

const EMPTY_SESSIONS: TaskSession[] = [];

/**
 * Plugin extension point in the chat input toolbar, rendered alongside the
 * first-party controls (model picker, mic, send). Renders every plugin
 * component registered for the `chat-input-actions` slot (each isolated behind
 * its own error boundary via `PluginSlot`) and forwards the current task and
 * all of its session ids as `slotProps`.
 */
export function ChatInputPluginActions(props: {
  sessionId: string | null;
  taskId: string | null;
  taskTitle?: string;
}) {
  const { sessionId, taskId, taskTitle } = props;
  // itemsByTaskId holds a stable per-task array reference (updated only when
  // that task's sessions change), so selecting it avoids a new-array-per-render.
  // Read optionally: the composer always renders under a StateProvider in the
  // app, but rendering the toolbar in isolation (unit tests) must not crash.
  const selectSessions = useCallback(
    (s: AppState): TaskSession[] =>
      taskId ? (s.taskSessionsByTask.itemsByTaskId[taskId] ?? EMPTY_SESSIONS) : EMPTY_SESSIONS,
    [taskId],
  );
  const taskSessions = useOptionalAppStore(selectSessions, EMPTY_SESSIONS);

  const slotProps = useMemo<ChatInputActionsSlotProps>(() => {
    const sessionIds: string[] = taskSessions.map((session) => session.id);
    // The active session may not yet be in the store list (freshly prepared);
    // make sure the plugin always receives it.
    if (sessionId && !sessionIds.includes(sessionId)) sessionIds.unshift(sessionId);
    return { taskId, taskTitle, activeSessionId: sessionId, sessionIds };
  }, [taskSessions, sessionId, taskId, taskTitle]);

  return <PluginSlot name="chat-input-actions" slotProps={slotProps} />;
}
