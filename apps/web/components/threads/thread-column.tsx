"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { IconArrowsMaximize } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import type { ActiveThread } from "@/lib/threads/active-threads";
import type { TaskSession } from "@/lib/types/http";
import { useTranslation } from "react-i18next";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { selectThreadSessionId } from "@/lib/threads/thread-session-selection";
import { resolveThreadColumnStatus, type ThreadStatus } from "@/lib/threads/thread-session-status";
import { ThreadConversation } from "./thread-conversation";
import { ThreadSessionStatusIcon, ThreadSessionSwitcher } from "./thread-session-switcher";

export function resolveThreadStatus(thread: ActiveThread): ThreadStatus {
  return resolveThreadColumnStatus({
    taskState: thread.taskState,
    reviewStatus: thread.reviewStatus,
    taskPendingAction: thread.taskPendingAction,
    session: {
      state: thread.sessionState,
      pending_action: thread.pendingAction,
    },
  });
}

function statusSessionForThread(thread: ActiveThread, selectedSession: TaskSession | null) {
  if (!selectedSession) {
    return {
      state: thread.sessionState,
      pending_action: thread.pendingAction,
    };
  }

  let pendingAction = selectedSession.pending_action;
  if (pendingAction === undefined) {
    pendingAction = selectedSession.id === thread.sessionId ? thread.pendingAction : null;
  }
  return {
    state: selectedSession.state,
    pending_action: pendingAction,
    foreground_activity: selectedSession.foreground_activity,
  };
}

function ThreadMeta({
  thread,
  status,
  sessions,
  selectedSessionId,
  onSelectSession,
}: {
  thread: ActiveThread;
  status: ThreadStatus;
  sessions: readonly TaskSession[];
  selectedSessionId: string | null;
  onSelectSession: (sessionId: string) => void;
}) {
  const { t } = useTranslation();
  const statusLabel = t(status.labelKey);
  return (
    <div className="flex min-w-0 items-center gap-2 text-[11px] text-muted-foreground">
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1">
        <span className="font-medium text-foreground/70">{statusLabel}</span>
        <span className="truncate">{thread.workflowName}</span>
        {thread.stepTitle && <span className="truncate">{thread.stepTitle}</span>}
        {thread.activeSubagentCount > 0 && (
          <Badge variant="secondary" className="h-4 px-1.5 text-[10px] font-normal">
            {t("threads:subagentCount", { count: thread.activeSubagentCount })}
          </Badge>
        )}
        {thread.queuedPromptCount > 0 && (
          <Badge variant="outline" className="h-4 px-1.5 text-[10px] font-normal">
            {t("threads:queuedPromptCount", { count: thread.queuedPromptCount })}
          </Badge>
        )}
      </div>
      <ThreadSessionSwitcher
        sessions={sessions}
        selectedSessionId={selectedSessionId}
        onSelect={onSelectSession}
      />
    </div>
  );
}

/**
 * Brings a deep-linked column into view once.
 *
 * The effect is keyed on `isFocused` alone, and the column is keyed by task id,
 * so a column that only mounts when a later snapshot lands still scrolls, while
 * a column that merely re-renders with fresh messages does not yank the deck
 * back under the reader.
 */
function useScrollWhenFocused(isFocused: boolean) {
  const ref = useRef<HTMLElement>(null);
  useEffect(() => {
    if (!isFocused) return;
    ref.current?.scrollIntoView({ inline: "center", block: "nearest", behavior: "smooth" });
  }, [isFocused]);
  return ref;
}

function ThreadSessionMembership({
  taskId,
  requestedSessionId,
  selectedSessionId,
  onSelectSession,
  onSessions,
  onRequestedSessionResolved,
  onInvalidRequestedSession,
}: {
  taskId: string;
  requestedSessionId: string | null;
  selectedSessionId: string | null;
  onSelectSession: (sessionId: string | null) => void;
  onSessions: (sessions: TaskSession[], isLoaded: boolean) => void;
  onRequestedSessionResolved: () => void;
  onInvalidRequestedSession?: (taskId: string, sessionId: string) => void;
}) {
  const { t } = useTranslation();
  const { sessions, isLoading, isLoaded, error, loadSessions } = useTaskSessions(taskId);
  const invalidRequestReportedRef = useRef<string | null>(null);

  const resolvedSessionId = selectThreadSessionId(sessions, {
    requestedSessionId,
    currentSessionId: selectedSessionId,
  });

  useEffect(() => {
    onSessions(sessions, isLoaded);
  }, [isLoaded, onSessions, sessions]);

  useEffect(() => {
    if (resolvedSessionId === selectedSessionId) return;
    onSelectSession(resolvedSessionId);
  }, [onSelectSession, resolvedSessionId, selectedSessionId]);

  useEffect(() => {
    if (!isLoaded || !requestedSessionId) return;
    onRequestedSessionResolved();
    if (sessions.some((session) => session.id === requestedSessionId)) return;
    if (invalidRequestReportedRef.current === requestedSessionId) return;
    invalidRequestReportedRef.current = requestedSessionId;
    onInvalidRequestedSession?.(taskId, requestedSessionId);
  }, [
    isLoaded,
    onInvalidRequestedSession,
    onRequestedSessionResolved,
    requestedSessionId,
    sessions,
    taskId,
  ]);

  if (isLoading) {
    return (
      <div
        className="flex items-center px-3 py-2 text-xs text-muted-foreground"
        data-testid="thread-session-list-loading"
      >
        {t("threads:sessionListLoading")}
      </div>
    );
  }

  if (error) {
    return (
      <div
        className="flex items-center gap-2 px-3 py-2 text-xs text-muted-foreground"
        data-testid="thread-session-list-error"
      >
        <span>{t("threads:sessionListUnavailable")}</span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 cursor-pointer px-2"
          onClick={() => void loadSessions(true)}
        >
          {t("threads:retrySessionList")}
        </Button>
      </div>
    );
  }

  if (sessions.length === 0) {
    return (
      <div
        className="flex items-center px-3 py-2 text-xs text-muted-foreground"
        data-testid="thread-session-list-empty"
      >
        {t("threads:sessionListEmpty")}
      </div>
    );
  }

  return null;
}

type ThreadColumnProps = {
  thread: ActiveThread;
  isFocused?: boolean;
  isPreloaded?: boolean;
  isDetailActive?: boolean;
  requestedSessionId?: string | null;
  onColumnRef?: (taskId: string, element: HTMLElement | null) => void;
  onInvalidRequestedSession?: (taskId: string, sessionId: string) => void;
  onOpenTask: (taskId: string) => void;
};

function ThreadColumnHeader({
  thread,
  status,
  sessions,
  selectedSessionId,
  onSelectSession,
  onOpenTask,
}: {
  thread: ActiveThread;
  status: ThreadStatus;
  sessions: readonly TaskSession[];
  selectedSessionId: string | null;
  onSelectSession: (sessionId: string) => void;
  onOpenTask: (taskId: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <header className="flex flex-col gap-1 border-b px-3 py-2">
      <div className="flex items-start gap-2">
        <ThreadSessionStatusIcon
          status={status}
          label={t(status.labelKey)}
          testId={`thread-status-${status.kind}`}
        />
        <p className="min-w-0 flex-1 truncate text-sm font-medium" title={thread.title}>
          {thread.title}
        </p>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0 cursor-pointer"
          aria-label={t("threads:openTask")}
          onClick={() => onOpenTask(thread.taskId)}
        >
          <IconArrowsMaximize className="h-3.5 w-3.5" />
        </Button>
      </div>
      <ThreadMeta
        thread={thread}
        status={status}
        sessions={sessions}
        selectedSessionId={selectedSessionId}
        onSelectSession={onSelectSession}
      />
    </header>
  );
}

function ThreadColumnBody({
  taskId,
  isPreloaded,
  isDetailActive,
  requestedSessionId,
  selectedSessionId,
  selectedSession,
  sessionListReady,
  onSelectSession,
  onSessions,
  onRequestedSessionResolved,
  onInvalidRequestedSession,
}: {
  taskId: string;
  isPreloaded: boolean;
  isDetailActive: boolean;
  requestedSessionId: string | null;
  selectedSessionId: string | null;
  selectedSession: TaskSession | null;
  sessionListReady: boolean;
  onSelectSession: (sessionId: string | null) => void;
  onSessions: (sessions: TaskSession[], isLoaded: boolean) => void;
  onRequestedSessionResolved: () => void;
  onInvalidRequestedSession?: (taskId: string, sessionId: string) => void;
}) {
  return (
    <div className="min-h-0 flex-1">
      {isPreloaded && (
        <ThreadSessionMembership
          taskId={taskId}
          requestedSessionId={requestedSessionId}
          selectedSessionId={selectedSessionId}
          onSelectSession={onSelectSession}
          onSessions={onSessions}
          onRequestedSessionResolved={onRequestedSessionResolved}
          onInvalidRequestedSession={onInvalidRequestedSession}
        />
      )}
      {isDetailActive && sessionListReady && selectedSession && (
        <ThreadConversation
          key={selectedSession.id}
          taskId={taskId}
          sessionId={selectedSession.id}
        />
      )}
    </div>
  );
}

export function ThreadColumn({
  thread,
  isFocused = false,
  isPreloaded = false,
  isDetailActive = false,
  requestedSessionId = null,
  onColumnRef,
  onInvalidRequestedSession,
  onOpenTask,
}: ThreadColumnProps) {
  const { t } = useTranslation();
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(
    () => requestedSessionId,
  );
  const [sessions, setSessions] = useState<TaskSession[]>([]);
  const [sessionListReady, setSessionListReady] = useState(false);
  const [requestedSessionResolved, setRequestedSessionResolved] = useState(
    () => !requestedSessionId,
  );
  const requestedSessionRef = useRef(requestedSessionId);
  const selectedSession = sessions.find((session) => session.id === selectedSessionId) ?? null;
  const statusSession = statusSessionForThread(thread, selectedSession);
  const status = resolveThreadColumnStatus({
    taskState: thread.taskState,
    reviewStatus: thread.reviewStatus,
    taskPendingAction: thread.taskPendingAction,
    session: statusSession,
  });

  useEffect(() => {
    if (requestedSessionRef.current === requestedSessionId) return;
    requestedSessionRef.current = requestedSessionId;
    if (requestedSessionId) {
      setSelectedSessionId(requestedSessionId);
      setRequestedSessionResolved(false);
    }
  }, [requestedSessionId]);

  const handleSelectSession = useCallback((sessionId: string | null) => {
    setSelectedSessionId(sessionId);
  }, []);
  const handleSessions = useCallback((nextSessions: TaskSession[]) => {
    setSessions(nextSessions);
  }, []);
  const handleSessionListState = useCallback(
    (nextSessions: TaskSession[], isLoaded: boolean) => {
      handleSessions(nextSessions);
      setSessionListReady(isLoaded);
    },
    [handleSessions],
  );
  const handleRequestedSessionResolved = useCallback(() => {
    setRequestedSessionResolved(true);
  }, []);
  const handleInvalidRequestedSession = useCallback(
    (taskId: string, sessionId: string) => {
      onInvalidRequestedSession?.(taskId, sessionId);
    },
    [onInvalidRequestedSession],
  );
  const focusRef = useScrollWhenFocused(isFocused);
  const setColumnRef = useCallback(
    (element: HTMLElement | null) => {
      focusRef.current = element;
      onColumnRef?.(thread.taskId, element);
    },
    [focusRef, onColumnRef, thread.taskId],
  );

  return (
    <section
      ref={setColumnRef}
      data-thread-column-id={thread.taskId}
      data-testid={`thread-column-${thread.taskId}`}
      data-focused={isFocused ? "true" : undefined}
      aria-label={t("threads:columnLabel", { title: thread.title })}
      // Phone: one column fills the viewport and snaps, so the deck is paged
      // instead of pinch-scrolled.
      //
      // Desktop: columns share the width rather than taking a fixed slice, so
      // two threads fill the board instead of leaving it mostly empty. The min
      // width is the floor they stop shrinking at, which is what turns a busy
      // deck into a horizontal scroll rather than a row of slivers.
      //
      // Two marks, deliberately different properties so they can coexist
      // without fighting over one ring colour:
      //   ring    — where the caret is. A composer's own border tracks agent
      //             state, not focus, so in a deck of composers nothing else
      //             says where typing would land.
      //   outline — the column a deep link asked for.
      className="flex h-full min-h-0 w-[85vw] shrink-0 snap-start flex-col overflow-hidden rounded-lg border bg-card focus-within:ring-2 focus-within:ring-ring data-[focused=true]:outline data-[focused=true]:outline-2 data-[focused=true]:outline-offset-2 data-[focused=true]:outline-primary md:w-auto md:min-w-[360px] md:flex-1 md:shrink"
    >
      <ThreadColumnHeader
        thread={thread}
        status={status}
        sessions={sessions}
        selectedSessionId={selectedSessionId}
        onSelectSession={handleSelectSession}
        onOpenTask={onOpenTask}
      />
      <ThreadColumnBody
        taskId={thread.taskId}
        isPreloaded={isPreloaded}
        isDetailActive={isDetailActive}
        requestedSessionId={requestedSessionResolved ? null : requestedSessionId}
        selectedSessionId={selectedSessionId}
        selectedSession={selectedSession}
        sessionListReady={sessionListReady}
        onSelectSession={handleSelectSession}
        onSessions={handleSessionListState}
        onRequestedSessionResolved={handleRequestedSessionResolved}
        onInvalidRequestedSession={handleInvalidRequestedSession}
      />
    </section>
  );
}
