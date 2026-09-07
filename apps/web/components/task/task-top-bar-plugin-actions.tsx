"use client";

import { memo, useCallback, useMemo, useRef } from "react";
import { useOptionalAppStore } from "@/components/state-provider";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

/**
 * Props forwarded to every plugin component registered for the `chat-top-bar`
 * slot (`registry.registerComponent("chat-top-bar", Component)`). This is the
 * session top bar's right-hand cluster, beside the document / editors / debug
 * controls — the place for at-a-glance status a
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
const MemoizedPluginSlot = memo(PluginSlot);

function sameStringArray(left: string[], right: string[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index]);
}

function useStableSessionIds(
  taskSessions: TaskSession[],
  activeSessionId: string | null,
): string[] {
  const sessionIdsRef = useRef<string[]>([]);
  return useMemo(() => {
    const nextSessionIds: string[] = taskSessions.map((session) => session.id);
    if (activeSessionId && !nextSessionIds.includes(activeSessionId)) {
      nextSessionIds.unshift(activeSessionId);
    }
    if (sameStringArray(sessionIdsRef.current, nextSessionIds)) return sessionIdsRef.current;
    sessionIdsRef.current = nextSessionIds;
    return nextSessionIds;
  }, [taskSessions, activeSessionId]);
}

/**
 * Plugin extension point in the session top bar, rendered alongside the
 * first-party controls (document/editor menus and debug toggle).
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
  const sessionIds = useStableSessionIds(taskSessions, sessionId);

  const slotProps = useMemo<ChatTopBarSlotProps>(() => {
    return { taskId, taskTitle, workspaceId, activeSessionId: sessionId, sessionIds };
  }, [sessionId, sessionIds, taskId, taskTitle, workspaceId]);

  return <MemoizedPluginSlot name="chat-top-bar" slotProps={slotProps} />;
}
