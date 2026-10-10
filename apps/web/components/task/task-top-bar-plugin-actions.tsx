"use client";

import { memo, useCallback, useMemo, useRef } from "react";
import { useOptionalAppStore } from "@/components/state-provider";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { ChatTopBarSlotProps } from "@/lib/plugins/types";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

export type { ChatTopBarSlotProps } from "@/lib/plugins/types";

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
  presentation?: ChatTopBarSlotProps["presentation"];
}) {
  const { sessionId, taskId, taskTitle, workspaceId, presentation = "desktop" } = props;
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
    return {
      taskId,
      taskTitle,
      workspaceId,
      activeSessionId: sessionId,
      sessionIds,
      presentation,
    };
  }, [presentation, sessionId, sessionIds, taskId, taskTitle, workspaceId]);

  const actionSurface = useMemo(
    () => ({ surface: "topbar" as const, presentation }),
    [presentation],
  );

  const content = (
    <MemoizedPluginSlot name="chat-top-bar" slotProps={slotProps} actionSurface={actionSurface} />
  );
  if (presentation === "desktop") return content;

  return (
    <div
      className="flex min-w-0 max-w-full flex-wrap items-center gap-2 overflow-x-clip [&>*]:min-w-0 [&>*]:max-w-full [&_[data-slot=button]]:!min-h-11 [&_[data-slot=button]]:!min-w-11 [&_[data-slot=button]]:!max-w-full [&_[data-slot=button]]:!whitespace-normal"
      data-testid="mobile-chat-top-bar-plugin-actions"
    >
      {content}
    </div>
  );
}

/** Reactively reports whether the phone menu needs a session-plugin section. */
export function useHasTaskTopBarPluginActions(): boolean {
  return usePluginRegistry().getSlotRegistrations("chat-top-bar").length > 0;
}
