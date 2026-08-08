"use client";

import { memo, useCallback, type ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { IconAlertCircle, IconX } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import { TurnGroupMessage } from "@/components/task/chat/messages/turn-group-message";
import { PrepareProgress } from "@/components/session/prepare-progress";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { dismissLastAgentError } from "@/lib/api/domains/session-api";
import { RemediationLink } from "@/components/task/remediation-link";
import {
  type LastAgentError,
  lastAgentErrorStamp,
  readLastAgentError,
} from "@/lib/session-last-agent-error";
import { useTranslation } from "react-i18next";

export type MessageListProps = {
  items: RenderItem[];
  messages: Message[];
  /** Action messages rendered after the env prep error status in the footer. */
  footerActionMessages?: Message[];
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId: Map<string, Message[]>;
  taskId?: string;
  sessionId: string | null;
  messagesLoading: boolean;
  isWorking: boolean;
  sessionState?: TaskSessionState;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  /** Render item key (see getItemKey) the unread "New" divider should
   *  appear immediately before; null/undefined renders no divider. */
  dividerBeforeItemKey?: string | null;
  /** Id of the most recent user-sent message, when known. Drives the
   * scroll-to-last-prompt button (always active) and the anchored-bar
   * affordance (opt-in, desktop only). */
  lastPromptMessageId?: string | null;
  /** Called whenever the last prompt's position relative to the transcript
   * viewport changes: fully `"above"` it (scrolled past, further down the
   * transcript), fully `"below"` it (not yet reached, e.g. browsing earlier
   * history), or still `"visible"` (some intersection remains). */
  onLastPromptEdgeChange?: (edge: LastPromptEdge) => void;
  /** Id of the earliest user-sent message, when known. Drives the
   * scroll-to-start button. */
  firstMessageId?: string | null;
  /** Called whenever the first message stops being fully visible (`true`) or
   * becomes fully visible again (`false`). */
  onFirstMessageHiddenChange?: (isHidden: boolean) => void;
  /** Rendered as the first child inside the scroll container, sticky at its
   * top — the desktop-only, opt-in anchored last-prompt bar. `null`/`undefined`
   * when the setting is off or on mobile. */
  stickyPromptBar?: ReactNode;
  /** Current rendered height (px) of the anchored last-prompt bar's pinned
   * overlay, or 0/undefined when it isn't showing. Lets a target scrolled
   * to the top of the transcript (e.g. the unread "New" divider) reserve
   * room for the overlay instead of being covered by it. */
  anchoredBarHeight?: number;
};

/** Imperative handle exposed by `MessageList`, letting the chat panel scroll
 * to an arbitrary message (e.g. the last
 * prompt) from outside the list — from the composer's scroll-up button. */
export type MessageListHandle = {
  scrollToMessage: (messageId: string, options?: { align?: "start" | "center" }) => void;
};

export function getItemKey(item: RenderItem): string {
  if (
    item.type === "turn_group" ||
    item.type === "prepare_progress" ||
    item.type === "agent_error_notice"
  )
    return item.id;
  return item.message.id;
}

export function getEffectiveActiveTurnId(
  activeTurnId: string | null,
  isWorking: boolean,
): string | null {
  return isWorking ? activeTurnId : null;
}

/** Index of the most recent user-authored message, or -1 when there is none. */
function findLastUserMessageIndex(messages: Message[]): number {
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].author_type === "user") return i;
  }
  return -1;
}

/** Id of the most recent user-authored message — the "last prompt" the user
 * sent. Used to power the transcript's scroll-to-last-prompt affordances. */
export function getLastUserMessageId(messages: Message[]): string | null {
  const index = findLastUserMessageIndex(messages);
  return index >= 0 ? messages[index].id : null;
}

/** Id of the earliest user-authored message — the "first prompt" (usually
 * the task description). Used to power the transcript's scroll-to-start
 * affordance. */
export function getFirstUserMessageId(messages: Message[]): string | null {
  const first = messages.find((message) => message.author_type === "user");
  return first ? first.id : null;
}

export type TranscriptNavigationTarget = "last_prompt" | "start";

/**
 * Whether resolving a transcript-navigation target needs another older page.
 * The latest prompt may sit before a long agent response; the true start can
 * only be known after the pagination cursor is exhausted.
 */
export function shouldLoadMoreForTranscriptTarget(
  target: TranscriptNavigationTarget,
  messages: Message[],
  hasMore: boolean,
): boolean {
  if (!hasMore) return false;
  return target === "start" || getLastUserMessageId(messages) === null;
}

/**
 * Whether the transcript's auto-follow-bottom behavior should force a scroll
 * to the bottom right now. `false` whenever a user-initiated programmatic
 * scroll (scroll-to-start / scroll-to-last-prompt) is still in flight or a
 * layout-rebuild restore is pending — otherwise a message streaming in
 * mid-scroll would silently snap the transcript back down and cancel the
 * user's action.
 */
export function shouldAutoScrollToBottom(params: {
  isNearBottom: boolean;
  isProgrammaticScrollLocked: boolean;
  hasPendingLayoutRestore: boolean;
}): boolean {
  return (
    params.isNearBottom && !params.isProgrammaticScrollLocked && !params.hasPendingLayoutRestore
  );
}

/** Where the last prompt sits relative to the transcript viewport: fully
 * `"above"` it (scrolled past going down), fully `"below"` it (not yet
 * reached, e.g. still browsing earlier history), or `"visible"` (some
 * intersection remains). The two-pixel tolerance avoids flickering at
 * fractional layout boundaries while scrolling settles. */
export type LastPromptEdge = "above" | "below" | "visible";

export function resolveLastPromptEdge(container: HTMLElement, target: HTMLElement): LastPromptEdge {
  const containerRect = container.getBoundingClientRect();
  const targetRect = target.getBoundingClientRect();
  const tolerance = 2;
  if (targetRect.bottom < containerRect.top - tolerance) return "above";
  if (targetRect.top > containerRect.bottom + tolerance) return "below";
  return "visible";
}

/** Derives the anchored bar's visibility and the always-on scroll button's
 * eligibility/direction from the last prompt's edge state. The anchored bar
 * only ever represents "you scrolled past it going down" (`above`) — it
 * must stay closed while the prompt is merely below the viewport because
 * the user hasn't reached it yet (e.g. browsing earlier history). The
 * scroll button instead stays eligible in both out-of-view states, but
 * points toward wherever the prompt actually is. */
export function resolveLastPromptControls(edge: LastPromptEdge): {
  anchoredBarVisible: boolean;
  scrollButtonEligible: boolean;
  scrollDirection: "up" | "down";
} {
  return {
    anchoredBarVisible: edge === "above",
    scrollButtonEligible: edge !== "visible",
    scrollDirection: edge === "below" ? "down" : "up",
  };
}

/** True when `target` sits entirely within `container`'s visible viewport —
 * neither edge clipped above or below. Powers the "scroll to start" button's
 * visibility, which appears as soon as the first prompt is even partially
 * clipped. */
export function isElementFullyVisible(container: HTMLElement, target: HTMLElement): boolean {
  const c = container.getBoundingClientRect();
  const t = target.getBoundingClientRect();
  const tolerance = 0.5;
  return t.top >= c.top - tolerance && t.bottom <= c.bottom + tolerance;
}

/** Pixel offset to reserve at the top of the transcript for the anchored
 * last-prompt bar's pinned overlay, so a scroll-into-view target (namely
 * the unread "New" divider) lands below it instead of underneath it.
 * Clamps an unmeasured/negative height to zero and rounds to whole pixels
 * — callers feed this straight into a CSS `scroll-margin-top`. */
export function anchoredBarScrollOffsetPx(anchoredBarHeight: number | undefined): number {
  return Math.max(0, Math.round(anchoredBarHeight ?? 0));
}

/** Only the latest ordinary agent row in the active turn can be the streaming reply. */
export function getStreamingAgentMessageId(messages: Message[]): string | null {
  const latestUserIndex = findLastUserMessageIndex(messages);
  for (let i = messages.length - 1; i > latestUserIndex; i--) {
    const message = messages[i];
    if (
      message.author_type === "agent" &&
      (message.type === "message" || message.type === "content" || !message.type)
    ) {
      return message.id;
    }
  }
  return null;
}

export function getLastTurnGroupId(items: RenderItem[]) {
  for (let i = items.length - 1; i >= 0; i--) {
    const item = items[i];
    if (item.type === "turn_group") return item.id;
  }
  return null;
}

export function getConversationLoadingState(params: {
  messagesLoading: boolean;
  messagesCount: number;
  isWorking: boolean;
  sessionState?: TaskSessionState | null;
}) {
  const isInitialLoading = params.messagesLoading && params.messagesCount === 0;
  const isNonLoadableSession =
    !params.sessionState ||
    params.sessionState === "CREATED" ||
    params.sessionState === "FAILED" ||
    params.sessionState === "COMPLETED" ||
    params.sessionState === "CANCELLED";
  // CREATED sessions are prepare-only: the agent hasn't been started yet, so
  // there is no conversation to load and the "Start agent" button is the
  // primary CTA. Suppress the spinner unconditionally to avoid a misleading
  // overlay racing with that button (synthetic task-description counts as a
  // message and would otherwise trip the messagesCount > 0 branch).
  if (params.sessionState === "CREATED") {
    return { isInitialLoading, showLoadingState: false };
  }
  return {
    isInitialLoading,
    showLoadingState:
      params.messagesLoading &&
      !params.isWorking &&
      (params.messagesCount > 0 || !isNonLoadableSession),
  };
}

/**
 * Decides whether message-list-native's initial divider-scroll effect may
 * (re-)apply `scrollIntoView` on the divider this render, versus leaving
 * whatever position the reader (or a prior correction) already settled on.
 *
 * True on the very first successful application (didScrollToDivider is
 * false) regardless of the other two gates — that first placement must
 * always happen once a divider target exists. After that, a re-assertion
 * (e.g. the transcript's initial data arriving in more than one wave, which
 * can shift where the divider actually lands) is only allowed while BOTH
 * the reader hasn't interacted yet (isUserScrolling — wheel, touch, or a
 * key press) AND the visit is still within its short settling window since
 * mount. Either gate tripping freezes the position for good: a live
 * message arriving well after the visit has genuinely settled — with no
 * interaction event to catch, e.g. a scrollbar drag — must never yank the
 * reader back to the divider.
 */
export function canReassertDividerScroll(params: {
  hasDividerTarget: boolean;
  didScrollToDivider: boolean;
  isUserScrolling: boolean;
  isWithinSettlingWindow: boolean;
}): boolean {
  if (!params.hasDividerTarget) return false;
  if (!params.didScrollToDivider) return true;
  return !params.isUserScrolling && params.isWithinSettlingWindow;
}

// The chat banner stays visible until the user explicitly dismisses it, even
// after the agent resumes — so the user can read the full error message at
// their own pace. The sidebar icon, by contrast, also auto-hides once the
// agent posts a new message (see agentErrorMessageForTask).
export function LastAgentErrorNotice({
  sessionId,
  error,
}: {
  sessionId: string | null;
  error: LastAgentError | null;
}) {
  const { t } = useTranslation();
  const stamp = error ? lastAgentErrorStamp(error) : "";
  const dismissedStamp = useAppStore((state) =>
    sessionId ? state.dismissedAgentErrors[sessionId] : undefined,
  );
  const dismissAgentError = useAppStore((state) => state.dismissAgentError);
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  const store = useAppStoreApi();

  const dismiss = useCallback(() => {
    if (!sessionId || !stamp) return;
    void dismissLastAgentError(sessionId, stamp)
      .then((resp) => {
        const current = readLastAgentError(
          store.getState().taskSessions.items[sessionId]?.metadata,
        );
        if (current && lastAgentErrorStamp(current) !== stamp) return;
        dismissAgentError(sessionId, stamp);
        setTaskSession(resp.session);
      })
      .catch((err: unknown) => {
        console.error("Failed to dismiss last agent error", err);
      });
  }, [dismissAgentError, sessionId, stamp, setTaskSession, store]);

  if (!error || dismissedStamp === stamp) return null;

  return (
    <div
      data-testid="last-agent-error-notice"
      className="mb-3 rounded-md border border-destructive/25 bg-destructive/10 text-destructive"
      role="alert"
    >
      <div className="flex items-start gap-2 px-3 py-2">
        <IconAlertCircle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <div className="text-xs font-medium">{t("task:previousAgentError")}</div>
          <pre className="mt-1 max-h-40 overflow-y-auto whitespace-pre-wrap break-words text-[11px] leading-relaxed text-destructive/85">
            {error.message}
          </pre>
          {error.remediationUrl && (
            <div className="mt-1">
              <RemediationLink url={error.remediationUrl} className="text-destructive/85" />
            </div>
          )}
        </div>
        <button
          type="button"
          className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded hover:bg-destructive/10 cursor-pointer"
          aria-label={t("task:hidePreviousAgentError")}
          onClick={dismiss}
        >
          <IconX className="h-3.5 w-3.5" aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}

/**
 * Slack-style unread-messages divider: a red rule with a "New" label,
 * positioned by the caller immediately before the first unread render item
 * (see hooks/use-processed-messages.ts's findUnreadDividerItemId).
 */
export function UnreadDivider() {
  const { t } = useTranslation();
  return (
    <div
      data-testid="unread-divider"
      role="separator"
      aria-label={t("task:newMessages")}
      className="relative my-3 flex items-center"
    >
      <div className="h-px flex-1 bg-destructive" />
      <span className="ml-2 shrink-0 text-[10px] font-semibold uppercase tracking-wide text-destructive">
        {t("task:new")}
      </span>
    </div>
  );
}

export function MessageListStatus({
  isLoadingMore,
  hasMore,
  showLoadingState,
  messagesLoading,
  isInitialLoading,
  messagesCount,
  onLoadMore,
}: {
  isLoadingMore: boolean;
  hasMore: boolean;
  showLoadingState: boolean;
  messagesLoading: boolean;
  isInitialLoading: boolean;
  messagesCount: number;
  /**
   * Explicitly load the previous page of older messages. Rendered as a button so
   * older history is always reachable even when the scroll-up IntersectionObserver
   * fails to re-arm (e.g. pinned at the very top with the sentinel always in view).
   */
  onLoadMore?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {isLoadingMore && hasMore && (
        <div className="text-center text-xs text-muted-foreground py-2">
          {t("task:loadingOlderMessages")}
        </div>
      )}
      {hasMore && !isLoadingMore && onLoadMore && (
        <div className="flex justify-center py-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="cursor-pointer text-xs text-muted-foreground"
            data-testid="load-older-messages"
            onClick={onLoadMore}
          >
            {t("task:loadOlderMessages")}
          </Button>
        </div>
      )}
      {showLoadingState && (
        <div
          className="flex items-center justify-center py-8 text-muted-foreground"
          data-testid="conversation-loading-state"
        >
          <GridSpinner className="text-primary mr-2" />
          <span>{t("task:loadingConversation")}</span>
        </div>
      )}
      {!messagesLoading && !isInitialLoading && messagesCount === 0 && (
        <div className="flex items-center justify-center py-8 text-muted-foreground">
          <span>{t("task:noMessagesYetStartTheConversation")}</span>
        </div>
      )}
    </>
  );
}

export const MessageItem = memo(function MessageItem({
  item,
  sessionId,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  worktreePath,
  onOpenFile,
  isLastGroup,
  activeTurnId,
  streamingMessageId,
  onScrollToMessage,
}: {
  item: RenderItem;
  sessionId: string | null;
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId: Map<string, Message[]>;
  taskId?: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  isLastGroup: boolean;
  activeTurnId?: string | null;
  streamingMessageId?: string | null;
  onScrollToMessage: (id: string) => void;
}) {
  if (item.type === "prepare_progress") {
    return <PrepareProgress sessionId={item.sessionId} />;
  }
  if (item.type === "agent_error_notice") {
    return <LastAgentErrorNotice sessionId={item.sessionId} error={item.error} />;
  }
  if (item.type === "turn_group") {
    const isContainingTurnActive = Boolean(activeTurnId && item.turnId === activeTurnId);
    return (
      <TurnGroupMessage
        group={item}
        sessionId={sessionId}
        permissionsByToolCallId={permissionsByToolCallId}
        childrenByParentToolCallId={childrenByParentToolCallId}
        taskId={taskId}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
        isLastGroup={isLastGroup}
        isTurnActive={isContainingTurnActive}
        streamingMessageId={streamingMessageId}
        onScrollToMessage={onScrollToMessage}
      />
    );
  }
  const isContainingTurnActive = Boolean(activeTurnId && item.message.turn_id === activeTurnId);
  return (
    <MessageRenderer
      comment={item.message}
      isTaskDescription={item.message.id === "task-description"}
      taskId={taskId}
      permissionsByToolCallId={permissionsByToolCallId}
      childrenByParentToolCallId={childrenByParentToolCallId}
      worktreePath={worktreePath}
      sessionId={sessionId ?? undefined}
      isTurnActive={isContainingTurnActive && item.message.id === streamingMessageId}
      isContainingTurnActive={isContainingTurnActive}
      onOpenFile={onOpenFile}
      onScrollToMessage={onScrollToMessage}
    />
  );
});
