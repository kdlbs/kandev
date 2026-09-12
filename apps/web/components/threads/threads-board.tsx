"use client";

import {
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
  type ReactNode,
  type RefObject,
} from "react";
import { IconColumns } from "@tabler/icons-react";
import type { ActiveThread } from "@/lib/threads/active-threads";
import { useTranslation } from "react-i18next";
import { ThreadColumn } from "./thread-column";
import { useThreadColumnActivation } from "./use-thread-column-activation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { MobileThreadPicker } from "./mobile-thread-picker";
import { ThreadTaskActionsProvider } from "./thread-task-actions";
import { useThreadSelectionRecovery } from "./use-thread-selection-recovery";
import { resolveThreadLayout, type ThreadLayoutResult } from "./thread-layout";
import type { ThreadLayout } from "@/lib/state/slices/ui/thread-view-types";
import { cn } from "@/lib/utils";

type ThreadsBoardProps = {
  threads: ActiveThread[];
  isLoading?: boolean;
  layout?: ThreadLayout;
  autoHideComposer?: boolean;
  /** Column a deep link asked for; scrolled into view and ringed on arrival. */
  focusedTaskId?: string | null;
  /** Session a task-detail link asked the target column to select. */
  focusedSessionId?: string | null;
  /** Removes a target session query after the target column proves it invalid. */
  onInvalidRequestedSession?: (taskId: string, sessionId: string) => void;
  onOpenTask: (taskId: string) => void;
  /** Composes the page header with the viewport's active phone thread. */
  renderHeader?: (activeMobileTaskId: string | null, gridHeightFallback: boolean) => ReactNode;
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

function focusThreadPicker(event: Event, column: Element | undefined) {
  const trigger = column?.querySelector<HTMLButtonElement>('[data-testid="thread-picker-trigger"]');
  if (!trigger) return;
  event.preventDefault();
  trigger.focus({ preventScroll: true });
}

type BoardSize = { height: number; width: number } | null;

function useBoardContentSize(
  boardRef: RefObject<HTMLDivElement | null>,
  isEmpty: boolean,
  setSize: Dispatch<SetStateAction<BoardSize>>,
) {
  useLayoutEffect(() => {
    const board = boardRef.current;
    if (!board) setSize(null);
    if (!board || typeof ResizeObserver === "undefined") return;
    let disposed = false;
    const observer = new ResizeObserver((entries) => {
      if (disposed) return;
      const entry = entries[0];
      if (entry) {
        const { height, width } = entry.contentRect;
        setSize((previous) =>
          previous?.height === height && previous.width === width ? previous : { height, width },
        );
      }
    });
    observer.observe(board);
    return () => {
      disposed = true;
      observer.disconnect();
    };
  }, [boardRef, isEmpty, setSize]);
}

function useThreadBoardLayout(
  orderedIds: readonly string[],
  layout: ThreadLayout,
  isMobile: boolean,
  focusedTaskId: string | null,
) {
  const [size, setSize] = useState<BoardSize>(null);
  const composition = resolveThreadLayout({
    layout,
    isMobile,
    taskCount: orderedIds.length,
    contentHeight: size?.height ?? null,
  });
  const layoutKey = `${composition.layout}:${composition.rows}:${isMobile}:${size?.width}`;
  const activation = useThreadColumnActivation(orderedIds, focusedTaskId, layoutKey);
  useBoardContentSize(activation.boardRef, orderedIds.length === 0, setSize);
  const rememberThread = useThreadSelectionRecovery(
    orderedIds,
    activation.boardRef,
    isMobile,
    layoutKey,
  );
  return { ...activation, composition, layoutKey, rememberThread };
}

function boardLayoutStyle(composition: ThreadLayoutResult) {
  if (composition.layout !== "grid") return undefined;
  return {
    gridAutoFlow: "column",
    gridTemplateRows: `repeat(${composition.rows}, minmax(0, 1fr))`,
    gridTemplateColumns: `repeat(${composition.columns}, minmax(360px, 1fr))`,
  };
}

function useThreadPicker(
  boardRef: RefObject<HTMLDivElement | null>,
  isMobile: boolean,
  mobileTaskId: string | null,
  firstId: string | null,
) {
  const [open, setOpen] = useState(false);
  if (!isMobile && open) setOpen(false);
  const returnFocusTaskId = useRef<string | null>(null);
  const taskColumn = (taskId: string | null) =>
    Array.from(boardRef.current?.children ?? []).find(
      (element) => element.getAttribute("data-thread-column-id") === taskId,
    );
  function choose(taskId: string) {
    returnFocusTaskId.current = taskId;
    setOpen(true);
  }
  function select(taskId: string) {
    returnFocusTaskId.current = taskId;
    taskColumn(taskId)?.scrollIntoView({ inline: "start", block: "nearest", behavior: "instant" });
    setOpen(false);
  }
  function restoreFocus(event: Event) {
    focusThreadPicker(
      event,
      taskColumn(returnFocusTaskId.current) ?? taskColumn(mobileTaskId) ?? taskColumn(firstId),
    );
  }
  return { open, setOpen, choose, select, restoreFocus };
}

/**
 * The deck: every live agent conversation as its own column, scrolled
 * horizontally. Columns keep the order the selector gave them, so a thread the
 * reader is following does not jump while they read it.
 */
export function ThreadsBoard({
  threads,
  isLoading = false,
  layout = "columns",
  autoHideComposer = false,
  focusedTaskId = null,
  focusedSessionId = null,
  onInvalidRequestedSession,
  onOpenTask,
  renderHeader,
}: ThreadsBoardProps) {
  const { t } = useTranslation();
  const { markedTaskId, retire } = useRetiringFocusMark(focusedTaskId);
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const orderedIds = useMemo(() => threads.map((thread) => thread.taskId), [threads]);
  const {
    boardRef,
    registerColumn,
    preloadTaskIds,
    detailTaskIds,
    mobileTaskId,
    composition,
    layoutKey,
    rememberThread,
  } = useThreadBoardLayout(orderedIds, layout, isMobile, focusedTaskId);
  const picker = useThreadPicker(boardRef, isMobile, mobileTaskId, orderedIds[0] ?? null);

  return (
    <ThreadTaskActionsProvider boardRef={boardRef}>
      <div className="flex h-full min-h-0 min-w-0 flex-col">
        {renderHeader?.(mobileTaskId, composition.heightFallback)}
        {composition.heightFallback && (
          <p role="status" className="shrink-0 px-3 py-1 text-xs text-muted-foreground">
            {t("threads:gridHeightFallback")}
          </p>
        )}
        {threads.length === 0 ? (
          <div className="min-h-0 flex-1">
            {isLoading ? <ThreadsLoadingState /> : <ThreadsEmptyState />}
          </div>
        ) : (
          <div
            data-testid="threads-board"
            data-layout={composition.layout}
            ref={boardRef}
            onWheelCapture={retire}
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
            className={cn(
              "min-h-0 min-w-0 w-full flex-1 snap-x snap-mandatory overflow-x-auto overflow-y-hidden overscroll-x-contain md:gap-3 md:p-3 md:snap-none",
              composition.layout === "grid" ? "grid" : "flex",
            )}
            style={boardLayoutStyle(composition)}
          >
            {threads.map((thread) => (
              <ThreadColumn
                key={thread.taskId}
                thread={thread}
                autoHideComposer={autoHideComposer && !isMobile && isFinePointer}
                mobileNavigation={
                  isMobile
                    ? {
                        onChoose: () => picker.choose(thread.taskId),
                      }
                    : undefined
                }
                isFocused={thread.taskId === markedTaskId}
                layoutKey={layoutKey}
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
            open={picker.open}
            onOpenChange={picker.setOpen}
            onSelect={picker.select}
            onCloseAutoFocus={picker.restoreFocus}
          />
        )}
      </div>
    </ThreadTaskActionsProvider>
  );
}
