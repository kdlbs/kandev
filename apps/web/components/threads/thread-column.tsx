"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type HTMLAttributes,
  type RefObject,
} from "react";
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
import { ComposerDisclosureContext } from "@/components/task/chat/composer-disclosure";
import { useComposerDisclosure } from "@/components/task/chat/use-composer-disclosure";
import { ThreadSessionStatusIcon, ThreadSessionSwitcher } from "./thread-session-switcher";
import { ThreadTaskMenuButton, useThreadTaskContextMenu } from "./thread-task-actions";
import {
  MobileThreadColumnHeader,
  type MobileThreadNavigation,
} from "./mobile-thread-column-header";

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
 * Keeps an initial deep link reachable until the reader interacts with the deck.
 *
 * Board resizing can interrupt the initial smooth scroll. Reassert the target
 * after that reflow, but never on ordinary transcript updates.
 */
function useScrollWhenFocused(isFocused: boolean, layoutKey: string) {
  const ref = useRef<HTMLElement>(null);
  useEffect(() => {
    if (!isFocused) return;
    const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    ref.current?.scrollIntoView({
      inline: "center",
      block: "nearest",
      behavior: reducedMotion ? "instant" : "smooth",
    });
  }, [isFocused, layoutKey]);
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
  autoHideComposer?: boolean;
  mobileNavigation?: MobileThreadNavigation;
  isFocused?: boolean;
  layoutKey?: string;
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
  mobileNavigation,
}: {
  thread: ActiveThread;
  status: ThreadStatus;
  sessions: readonly TaskSession[];
  selectedSessionId: string | null;
  onSelectSession: (sessionId: string) => void;
  onOpenTask: (taskId: string) => void;
  mobileNavigation?: MobileThreadNavigation;
}) {
  const { t } = useTranslation();
  const onContextMenu = useThreadTaskContextMenu(thread.taskId);
  if (mobileNavigation) {
    return (
      <MobileThreadColumnHeader
        thread={thread}
        status={status}
        sessions={sessions}
        selectedSessionId={selectedSessionId}
        onSelectSession={onSelectSession}
        onOpenTask={onOpenTask}
        navigation={mobileNavigation}
      />
    );
  }
  return (
    <header className="flex flex-col gap-1 border-b px-3 py-2" onContextMenu={onContextMenu}>
      <div className="flex items-start gap-2">
        <ThreadSessionStatusIcon
          status={status}
          label={t(status.labelKey)}
          testId={`thread-status-${status.kind}`}
        />
        <p className="min-w-0 flex-1 truncate text-sm font-medium" title={thread.title}>
          {thread.title}
        </p>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 shrink-0 cursor-pointer [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
            aria-label={t("threads:openTask")}
            onClick={() => onOpenTask(thread.taskId)}
          >
            <IconArrowsMaximize className="h-3.5 w-3.5" />
          </Button>
          <ThreadTaskMenuButton taskId={thread.taskId} />
        </div>
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
    <div className="min-h-0 min-w-0 flex-1">
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

function useThreadComposerInteraction(
  enabled: boolean,
  sessionId: string | null,
  tileRef: RefObject<HTMLElement | null>,
) {
  const controller = useComposerDisclosure({ enabled, sessionId });
  const returningFocus = useRef(false);
  const disclosure = {
    ...controller,
    collapse: () => {
      if (!controller.canCollapse) return;
      controller.collapse();
      returningFocus.current = true;
      tileRef.current?.focus({ preventScroll: true });
      returningFocus.current = false;
    },
  };
  const bindings: HTMLAttributes<HTMLElement> = {
    tabIndex: enabled ? 0 : undefined,
    onPointerEnter: (event) => controller.pointerEnter(event.pointerType),
    onPointerLeave: (event) => controller.pointerLeave(event.pointerType),
    onFocusCapture: () => controller.focus(returningFocus.current),
    onBlurCapture: controller.blur,
    onKeyDown: (event) => {
      if (event.target === event.currentTarget && event.key === "Enter" && !event.repeat) {
        event.preventDefault();
        controller.reveal();
      }
    },
  };
  return { disclosure, bindings };
}

export function ThreadColumn({
  thread,
  autoHideComposer = false,
  mobileNavigation,
  isFocused = false,
  layoutKey = "columns",
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
  const handleSessionListState = useCallback((nextSessions: TaskSession[], isLoaded: boolean) => {
    setSessions(nextSessions);
    setSessionListReady(isLoaded);
  }, []);
  const handleRequestedSessionResolved = useCallback(() => {
    setRequestedSessionResolved(true);
  }, []);
  const focusRef = useScrollWhenFocused(isFocused, layoutKey);
  const composer = useThreadComposerInteraction(
    autoHideComposer,
    isDetailActive ? selectedSessionId : null,
    focusRef,
  );
  const setColumnRef = useCallback(
    (element: HTMLElement | null) => {
      focusRef.current = element;
      onColumnRef?.(thread.taskId, element);
    },
    [focusRef, onColumnRef, thread.taskId],
  );

  return (
    <ComposerDisclosureContext value={composer.disclosure}>
      <section
        {...composer.bindings}
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
        className="flex h-full min-h-0 min-w-0 w-full shrink-0 snap-start flex-col overflow-hidden bg-card focus-within:ring-2 focus-within:ring-ring data-[focused=true]:outline data-[focused=true]:outline-2 data-[focused=true]:outline-offset-[-2px] data-[focused=true]:outline-primary md:w-auto md:min-w-[360px] md:flex-1 md:shrink md:rounded-lg md:border md:data-[focused=true]:outline-offset-2"
      >
        <ThreadColumnHeader
          thread={thread}
          mobileNavigation={mobileNavigation}
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
          onInvalidRequestedSession={onInvalidRequestedSession}
        />
      </section>
    </ComposerDisclosureContext>
  );
}
