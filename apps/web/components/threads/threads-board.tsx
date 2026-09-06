"use client";

import { useMemo, useState } from "react";
import { IconColumns } from "@tabler/icons-react";
import type { ActiveThread } from "@/lib/threads/active-threads";
import { useTranslation } from "react-i18next";
import { ThreadColumn } from "./thread-column";
import { useThreadColumnActivation } from "./use-thread-column-activation";

type ThreadsBoardProps = {
  threads: ActiveThread[];
  isLoading?: boolean;
  /** Column a deep link asked for; scrolled into view and ringed on arrival. */
  focusedTaskId?: string | null;
  /** Session a task-detail link asked the target column to select. */
  focusedSessionId?: string | null;
  /** Removes a target session query after the target column proves it invalid. */
  onInvalidRequestedSession?: (taskId: string, sessionId: string) => void;
  onOpenTask: (taskId: string) => void;
};

function ThreadsPlaceholder({ testId, children }: { testId: string; children: React.ReactNode }) {
  return (
    <div
      data-testid={testId}
      className="flex h-full min-h-0 w-full flex-col items-center justify-center gap-2 px-6 text-center"
    >
      {children}
    </div>
  );
}

function ThreadsEmptyState() {
  const { t } = useTranslation();
  return (
    <ThreadsPlaceholder testId="threads-empty-state">
      <IconColumns aria-hidden="true" className="h-8 w-8 text-muted-foreground/50" />
      <p className="text-sm font-medium">{t("threads:emptyTitle")}</p>
      <p className="max-w-md text-sm text-muted-foreground">{t("threads:emptyBody")}</p>
    </ThreadsPlaceholder>
  );
}

function ThreadsLoadingState() {
  const { t } = useTranslation();
  return (
    <ThreadsPlaceholder testId="threads-loading-state">
      <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
        {t("threads:loading")}
      </p>
    </ThreadsPlaceholder>
  );
}

/**
 * The deep-link mark answers "where is the column I asked for", so it retires
 * the moment the reader starts using the deck rather than sitting on a column
 * they have since moved away from. A later deep link earns a fresh mark, which
 * is why dismissal is keyed to the requested id rather than latched forever.
 *
 * Uses the store-previous-props pattern instead of an effect so the mark never
 * paints for a frame after a new request has already been dismissed.
 */
function useRetiringFocusMark(focusedTaskId: string | null) {
  const [retired, setRetired] = useState(false);
  const [requested, setRequested] = useState(focusedTaskId);
  if (requested !== focusedTaskId) {
    setRequested(focusedTaskId);
    setRetired(false);
  }
  return {
    markedTaskId: retired ? null : focusedTaskId,
    retire: () => setRetired(true),
  };
}

/**
 * The deck: every live agent conversation as its own column, scrolled
 * horizontally. Columns keep the order the selector gave them, so a thread the
 * reader is following does not jump while they read it.
 */
export function ThreadsBoard({
  threads,
  isLoading = false,
  focusedTaskId = null,
  focusedSessionId = null,
  onInvalidRequestedSession,
  onOpenTask,
}: ThreadsBoardProps) {
  const { markedTaskId, retire } = useRetiringFocusMark(focusedTaskId);
  const orderedIds = useMemo(() => threads.map((thread) => thread.taskId), [threads]);
  const { boardRef, registerColumn, preloadTaskIds, detailTaskIds } = useThreadColumnActivation(
    orderedIds,
    focusedTaskId,
  );

  if (threads.length === 0) {
    return isLoading ? <ThreadsLoadingState /> : <ThreadsEmptyState />;
  }

  return (
    <div
      data-testid="threads-board"
      ref={boardRef}
      // Capture phase: a column's own handlers must not be able to swallow the
      // interaction that retires the mark.
      onPointerDownCapture={retire}
      onFocusCapture={retire}
      className="flex h-full min-h-0 w-full snap-x snap-mandatory gap-3 overflow-x-auto overflow-y-hidden p-3 md:snap-none"
    >
      {threads.map((thread) => (
        <ThreadColumn
          key={thread.taskId}
          thread={thread}
          isFocused={thread.taskId === markedTaskId}
          requestedSessionId={thread.taskId === focusedTaskId ? focusedSessionId : null}
          isPreloaded={preloadTaskIds.has(thread.taskId)}
          isDetailActive={detailTaskIds.has(thread.taskId)}
          onInvalidRequestedSession={onInvalidRequestedSession}
          onColumnRef={registerColumn}
          onOpenTask={onOpenTask}
        />
      ))}
    </div>
  );
}
