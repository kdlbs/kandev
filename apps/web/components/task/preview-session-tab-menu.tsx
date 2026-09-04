"use client";

import { useSessionActions } from "@/hooks/domains/session/use-session-actions";
import { shareableSessionStateClient } from "@/components/task/share/share-button";
import type { TaskSession } from "@/lib/types/http";
import { SessionContextMenuItems } from "./session-tab-menu";

/**
 * Per-session context menu for a kanban preview tab.
 *
 * Mounted once per session inside `SessionTabs`' `renderContextMenu` slot, so
 * it owns its own `useSessionActions` instance (stop/resume/setPrimary/delete)
 * scoped to that session. Confirmation dialogs (delete/share/handoff) have no
 * per-tab slot to render into, so they're hoisted to `PreviewSessionTabs` —
 * this component only requests them via the callbacks below.
 */
export function PreviewSessionTabMenu({
  session,
  taskId,
  isPrimary,
  onRename,
  onRequestDelete,
  onSessionRemoved,
  onShareRequested,
  onHandoff,
}: {
  session: TaskSession;
  taskId: string;
  isPrimary: boolean;
  onRename: (sessionId: string) => void;
  onRequestDelete: (sessionId: string, event: Event, confirmDelete: () => Promise<boolean>) => void;
  onSessionRemoved: (sessionId: string) => void;
  onShareRequested: (sessionId: string) => void;
  onHandoff: (sessionId: string, profileId: string) => void;
}) {
  const { setPrimary, stop, resume, remove } = useSessionActions({
    sessionId: session.id,
    taskId,
    onDeleted: () => onSessionRemoved(session.id),
  });

  return (
    <SessionContextMenuItems
      sessionState={session.state}
      isPrimary={isPrimary}
      canShare={shareableSessionStateClient(session.state)}
      taskId={taskId}
      sessionId={session.id}
      actions={{
        handleSetPrimary: setPrimary,
        handleStop: stop,
        handleResume: resume,
        hideSessionPanel: () => undefined,
      }}
      onDelete={(event) => onRequestDelete(session.id, event, () => remove({ feedback: "toast" }))}
      onShare={() => onShareRequested(session.id)}
      onHandoffProfile={(profileId) => onHandoff(session.id, profileId)}
      onStartRename={() => onRename(session.id)}
    />
  );
}
