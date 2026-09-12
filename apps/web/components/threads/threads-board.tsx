"use client";

import { useMemo, useRef, useState, type ReactNode } from "react";
import { IconColumns } from "@tabler/icons-react";
import type { ActiveThread } from "@/lib/threads/active-threads";
import { useTranslation } from "react-i18next";
import { ThreadColumn } from "./thread-column";
import { useThreadColumnActivation } from "./use-thread-column-activation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { MobileThreadPicker } from "./mobile-thread-picker";
import { ThreadTaskActionsProvider } from "./thread-task-actions";
import { useThreadSelectionRecovery } from "./use-thread-selection-recovery";

type ThreadsBoardProps = {
  threads: ActiveThread[];
  isLoading?: boolean;
  /** Column a deep link asked for; scrolled into view and ringed on arrival. */
  focusedTaskId?: string | null;
  /** URL request identity stays stable while its target is temporarily absent. */
  focusRequestKey?: string | null;
  /** Session a task-detail link asked the target column to select. */
  focusedSessionId?: string | null;
  /** Removes a target session query after the target column proves it invalid. */
  onInvalidRequestedSession?: (taskId: string, sessionId: string) => void;
  onOpenTask: (taskId: string) => void;
  /** Composes the page header with the viewport's active phone thread. */
  renderHeader?: (activeMobileTaskId: string | null) => ReactNode;
};

function ThreadsPlaceholder({ testId, children }: { testId: string; children: React.ReactNode }) {
  return (
    <div
      data-testid={testId}
      tabIndex={-1}
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
 * is why dismissal is keyed to the URL request rather than its resolved column.
 *
 * Uses the store-previous-props pattern instead of an effect so the mark never
 * paints for a frame after a new request has already been dismissed.
 */
function useRetiringFocusMark(focusedTaskId: string | null, focusRequestKey: string | null) {
  const [retired, setRetired] = useState(false);
  const [requested, setRequested] = useState(focusRequestKey);
  if (requested !== focusRequestKey) {
    setRequested(focusRequestKey);
    setRetired(false);
  }
  return {
    markedTaskId: retired ? null : focusedTaskId,
    retire: () => setRetired(true),
  };
}

function focusThreadPicker(event: Event, column: Element | undefined) {
  const trigger = column?.querySelector<HTMLButtonElement>('[data-testid="thread-picker-trigger"]');
  if (!trigger) return;
  event.preventDefault();
  trigger.focus({ preventScroll: true });
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
  focusRequestKey = focusedTaskId,
  focusedSessionId = null,
  onInvalidRequestedSession,
  onOpenTask,
  renderHeader,
}: ThreadsBoardProps) {
  const { markedTaskId, retire } = useRetiringFocusMark(focusedTaskId, focusRequestKey);
  const { isMobile } = useResponsiveBreakpoint();
  const [pickerOpen, setPickerOpen] = useState(false);
  if (!isMobile && pickerOpen) setPickerOpen(false);
  const returnFocusTaskId = useRef<string | null>(null);
  const orderedIds = useMemo(() => threads.map((thread) => thread.taskId), [threads]);
  const { boardRef, registerColumn, preloadTaskIds, detailTaskIds, mobileTaskId } =
    useThreadColumnActivation(orderedIds, markedTaskId);
  const rememberThread = useThreadSelectionRecovery(orderedIds, boardRef, isMobile);

  function taskColumn(taskId: string | null) {
    return Array.from(boardRef.current?.children ?? []).find(
      (element) => element.getAttribute("data-thread-column-id") === taskId,
    );
  }

  function selectThread(taskId: string) {
    returnFocusTaskId.current = taskId;
    taskColumn(taskId)?.scrollIntoView({ inline: "start", block: "nearest", behavior: "instant" });
    setPickerOpen(false);
  }

  function restorePickerFocus(event: Event) {
    const column =
      taskColumn(returnFocusTaskId.current) ??
      taskColumn(mobileTaskId) ??
      taskColumn(orderedIds[0] ?? null);
    focusThreadPicker(event, column);
  }

  return (
    <ThreadTaskActionsProvider boardRef={boardRef}>
      <div className="flex h-full min-h-0 min-w-0 flex-col">
        {renderHeader?.(mobileTaskId)}
        {threads.length === 0 ? (
          <div className="min-h-0 flex-1">
            {isLoading ? <ThreadsLoadingState /> : <ThreadsEmptyState />}
          </div>
        ) : (
          <div
            data-testid="threads-board"
            ref={boardRef}
            // Capture phase: a column's own handlers must not be able to swallow the
            // interaction that retires the mark.
            onPointerDownCapture={(event) => {
              retire();
              rememberThread(event);
            }}
            onFocusCapture={(event) => {
              retire();
              rememberThread(event);
            }}
            className="flex min-h-0 min-w-0 w-full flex-1 snap-x snap-mandatory overflow-x-auto overflow-y-hidden overscroll-x-contain md:gap-3 md:p-3 md:snap-none"
          >
            {threads.map((thread) => (
              <ThreadColumn
                key={thread.taskId}
                thread={thread}
                mobileNavigation={
                  isMobile
                    ? {
                        onChoose: () => {
                          returnFocusTaskId.current = thread.taskId;
                          setPickerOpen(true);
                        },
                      }
                    : undefined
                }
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
        )}
        {isMobile && threads.length > 0 && (
          <MobileThreadPicker
            threads={threads}
            selectedTaskId={mobileTaskId}
            open={pickerOpen}
            onOpenChange={setPickerOpen}
            onSelect={selectThread}
            onCloseAutoFocus={restorePickerFocus}
          />
        )}
      </div>
    </ThreadTaskActionsProvider>
  );
}
