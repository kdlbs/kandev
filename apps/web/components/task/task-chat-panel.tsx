"use client";

/* eslint-disable max-lines -- this component composes the complete transcript surface. */

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  memo,
  type MutableRefObject,
  type RefObject,
} from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import {
  type ChatInputContainerHandle,
  type ChatSubmitPayload,
  type ChatSubmitResult,
} from "@/components/task/chat/chat-input-container";
import { MessageList } from "@/components/task/chat/message-list";
import {
  type MessageListHandle,
  type LastPromptEdge,
  getLastUserMessageId,
  getFirstUserMessageId,
  resolveLastPromptControls,
} from "@/components/task/chat/message-list-shared";
import { AnchoredLastPromptBar } from "@/components/task/chat/anchored-last-prompt-bar";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useIsTaskArchived } from "./task-archived-context";
import { useChatPanelState } from "./chat/use-chat-panel-state";
import { ChatInputArea, useSubmitHandler, useChatPanelHandlers } from "./chat/chat-input-area";
import { useDockviewStore, type TranscriptScrollTarget } from "@/lib/state/dockview-store";
import { ClarificationPanelSection } from "./chat/clarification-panel-section";
import { useComposerAgentStartHint } from "./chat/use-composer-agent-start-hint";
import { PanelSearchBar } from "@/components/search/panel-search-bar";
import { SessionSearchHits } from "@/components/task/chat/session-search-hits";
import { usePanelSearch } from "@/hooks/use-panel-search";
import { useSessionSearch } from "@/hooks/domains/session/use-session-search";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { findUnreadDividerItemId, lastRenderedMessageId } from "@/lib/session-unread-divider";
import { useSessionReadTracking } from "./chat/use-session-read-tracking";
import { useDrainOlderMessages } from "@/components/task/chat/use-drain-older-messages";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { routePanelMouseDown } from "./chat/route-panel-mouse-down";
import { useTranslation } from "react-i18next";

import { loadMessageWindowAround } from "@/hooks/domains/session/load-message-window";
import { TaskChatLaunchError } from "./simple/components/task-chat-launch-error";
import { isTypedTaskLaunchError } from "./simple/components/task-launch-error-entry";
import { useTaskLaunchErrorContext } from "./task-launch-error-context";
import { useTaskStatusSummary } from "@/hooks/domains/task/use-task-status-summary";
import { isTaskLaunchErrorOwnedBySession } from "@/components/task/chat/types";
import { TaskMarkdownFileLinkProvider } from "@/components/shared/task-markdown-file-link-provider";

/** Returns a `clarificationKey` that increments each time a pending
 * clarification is resolved, letting the composer reset its input state for
 * the next clarification round. */
function useClarificationKey(agentMessageCount: number) {
  const lastCountRef = useRef(agentMessageCount);
  const [clarificationKey, setClarificationKey] = useState(0);
  useEffect(() => {
    lastCountRef.current = agentMessageCount;
  }, [agentMessageCount]);
  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);
  return { clarificationKey, handleClarificationResolved };
}

/** Identity for a prompt-history target owned by a non-Dockview host. */
export type PendingMessageScrollTarget = {
  sessionId: string;
  messageId: string;
  token: number;
  hostPanelId: string;
};

/** Scrolls a non-Dockview host target after the message row becomes rendered. */
type PendingMessageScrollOptions = {
  messageListRef: RefObject<MessageListHandle | null>;
  sessionId: string | null;
  messageId: string | null | undefined;
  target?: PendingMessageScrollTarget | null;
  onConsumed: ((messageId: string) => void) | undefined;
  readinessKey: string;
  isInitialMessagesLoading: boolean;
  isVisible?: boolean;
};

type MessageTargetLifecycle = {
  sessionId: string | null;
  messageId: string | null | undefined;
  target?: PendingMessageScrollTarget | null;
  isVisible: boolean;
};

type AroundWindowRequestOptions = {
  sessionId: string;
  messageId: string;
  targetKey: string;
  store: StoreApi<AppState>;
  requestKeysRef: MutableRefObject<Set<string>>;
  mountedRef: MutableRefObject<boolean>;
  isCurrentTarget: () => boolean;
  setLoading: (loading: boolean) => void;
  onMerged: () => void;
  onCancelled: () => void;
};

function requestMessageWindowAround({
  sessionId,
  messageId,
  targetKey,
  store,
  requestKeysRef,
  mountedRef,
  isCurrentTarget,
  setLoading,
  onMerged,
  onCancelled,
}: AroundWindowRequestOptions) {
  requestKeysRef.current.add(targetKey);
  setLoading(true);
  void loadMessageWindowAround(sessionId, messageId, isCurrentTarget, store)
    .then((result) => {
      if (!isCurrentTarget()) return;
      if (result.kind === "merged") onMerged();
      else if (result.kind === "deleted-target") onCancelled();
    })
    .catch(() => {
      if (isCurrentTarget()) onCancelled();
    })
    .finally(() => {
      requestKeysRef.current.delete(targetKey);
      if (mountedRef.current) setLoading(requestKeysRef.current.size > 0);
    });
}

type TargetReassertionOptions = {
  targetKey: string;
  timerRef: MutableRefObject<number | null>;
  attemptedRef: MutableRefObject<Set<string>>;
  completedAroundRef: MutableRefObject<Set<string>>;
  isCurrentTarget: () => boolean;
  scroll: () => void;
  consume: () => void;
};

function scheduleTargetReassertion({
  targetKey,
  timerRef,
  attemptedRef,
  completedAroundRef,
  isCurrentTarget,
  scroll,
  consume,
}: TargetReassertionOptions) {
  if (timerRef.current !== null || attemptedRef.current.has(targetKey) || !isCurrentTarget()) {
    return;
  }
  timerRef.current = window.setTimeout(() => {
    timerRef.current = null;
    if (!isCurrentTarget()) return;
    attemptedRef.current.add(targetKey);
    scroll();
    completedAroundRef.current.delete(targetKey);
    consume();
  }, 250);
}

function cancelTargetReassertion(timerRef: MutableRefObject<number | null>) {
  if (timerRef.current === null) return;
  window.clearTimeout(timerRef.current);
  timerRef.current = null;
}

type PendingMessageScrollEffectOptions = {
  messageListRef: RefObject<MessageListHandle | null>;
  target: PendingMessageScrollTarget | null | undefined;
  onConsumed: ((messageId: string) => void) | undefined;
  readinessKey: string;
  isInitialMessagesLoading: boolean;
  isVisible: boolean;
  effectiveSessionId: string | null;
  effectiveMessageId: string | null | undefined;
  effectiveTargetKey: string | null;
  targetBelongsToHost: boolean;
  store: StoreApi<AppState>;
  refs: {
    requestKeys: MutableRefObject<Set<string>>;
    completedAround: MutableRefObject<Set<string>>;
    targetIdentity: MutableRefObject<string | null>;
    scrollSucceededTarget: MutableRefObject<string | null>;
    reassertionTimer: MutableRefObject<number | null>;
    reassertionAttempted: MutableRefObject<Set<string>>;
    mounted: MutableRefObject<boolean>;
    lifecycle: MutableRefObject<MessageTargetLifecycle>;
  };
  setIsLoading: (loading: boolean) => void;
};
type PendingMessageTargetAttemptOptions = {
  messageListRef: RefObject<MessageListHandle | null>;
  sessionId: string;
  messageId: string;
  targetKey: string;
  isInitialMessagesLoading: boolean;
  store: StoreApi<AppState>;
  refs: PendingMessageScrollEffectOptions["refs"];
  setIsLoading: (loading: boolean) => void;
  isCurrentTarget: () => boolean;
  consume: () => void;
};

function attemptPendingMessageScroll({
  messageListRef,
  sessionId,
  messageId,
  targetKey,
  isInitialMessagesLoading,
  store,
  refs,
  setIsLoading,
  isCurrentTarget,
  consume,
}: PendingMessageTargetAttemptOptions) {
  if (!isCurrentTarget()) return;
  if (
    refs.completedAround.current.has(targetKey) &&
    (refs.reassertionTimer.current !== null || refs.reassertionAttempted.current.has(targetKey))
  ) {
    return;
  }
  const scheduleReassertion = () =>
    scheduleTargetReassertion({
      targetKey,
      timerRef: refs.reassertionTimer,
      attemptedRef: refs.reassertionAttempted,
      completedAroundRef: refs.completedAround,
      isCurrentTarget,
      scroll: () => {
        messageListRef.current?.scrollToMessage(messageId, {
          align: "start",
          behavior: "auto",
        });
      },
      consume,
    });
  if (
    messageListRef.current?.scrollToMessage(messageId, {
      align: "start",
      behavior: "auto",
    })
  ) {
    refs.scrollSucceededTarget.current = targetKey;
    if (refs.completedAround.current.has(targetKey)) scheduleReassertion();
    else if (!refs.requestKeys.current.has(targetKey)) consume();
    return;
  }
  if (isInitialMessagesLoading) return;
  const loaded = store
    .getState()
    .messages.bySession[sessionId]?.some((message) => message.id === messageId);
  if (
    loaded ||
    refs.requestKeys.current.has(targetKey) ||
    refs.completedAround.current.has(targetKey)
  ) {
    return;
  }
  requestMessageWindowAround({
    sessionId,
    messageId,
    targetKey,
    store,
    requestKeysRef: refs.requestKeys,
    mountedRef: refs.mounted,
    isCurrentTarget,
    setLoading: setIsLoading,
    onMerged: () => {
      refs.completedAround.current.add(targetKey);
      if (refs.scrollSucceededTarget.current === targetKey) scheduleReassertion();
    },
    onCancelled: consume,
  });
}

function usePendingMessageScrollEffect(options: PendingMessageScrollEffectOptions) {
  const {
    messageListRef,
    target,
    onConsumed,
    readinessKey,
    isInitialMessagesLoading,
    isVisible,
    effectiveSessionId,
    effectiveMessageId,
    effectiveTargetKey,
    targetBelongsToHost,
    store,
    refs,
    setIsLoading,
  } = options;
  useEffect(() => {
    const cancelReassertion = () => cancelTargetReassertion(refs.reassertionTimer);
    if (!targetBelongsToHost) {
      refs.targetIdentity.current = null;
      refs.completedAround.current.clear();
      cancelReassertion();
      setIsLoading(false);
      if (target) onConsumed?.(target.messageId);
      return;
    }
    if (!effectiveSessionId || !effectiveMessageId || !effectiveTargetKey) {
      refs.targetIdentity.current = null;
      refs.completedAround.current.clear();
      cancelReassertion();
      setIsLoading(false);
      return;
    }
    if (refs.targetIdentity.current !== effectiveTargetKey) {
      refs.targetIdentity.current = effectiveTargetKey;
      refs.scrollSucceededTarget.current = null;
      refs.completedAround.current.clear();
      cancelReassertion();
    }
    if (!isVisible) {
      cancelReassertion();
      refs.completedAround.current.delete(effectiveTargetKey);
      return;
    }
    const isCurrentTarget = () =>
      refs.mounted.current &&
      refs.lifecycle.current.isVisible &&
      refs.targetIdentity.current === effectiveTargetKey &&
      refs.lifecycle.current.sessionId === effectiveSessionId &&
      refs.lifecycle.current.messageId === effectiveMessageId;
    const consume = () => onConsumed?.(effectiveMessageId);
    const frame = requestAnimationFrame(() => {
      attemptPendingMessageScroll({
        messageListRef,
        sessionId: effectiveSessionId,
        messageId: effectiveMessageId,
        targetKey: effectiveTargetKey,
        isInitialMessagesLoading,
        store,
        refs,
        setIsLoading,
        isCurrentTarget,
        consume,
      });
    });
    return () => cancelAnimationFrame(frame);
  }, [
    effectiveMessageId,
    effectiveSessionId,
    effectiveTargetKey,
    isInitialMessagesLoading,
    isVisible,
    messageListRef,
    onConsumed,
    readinessKey,
    refs,
    setIsLoading,
    store,
    target,
    targetBelongsToHost,
  ]);
}

export function usePendingMessageScroll({
  messageListRef,
  sessionId,
  messageId,
  target,
  onConsumed,
  readinessKey,
  isInitialMessagesLoading,
  isVisible = true,
}: PendingMessageScrollOptions) {
  const store = useAppStoreApi();
  const [isLoading, setIsLoading] = useState(false);
  const refs = useMemo(
    () => ({
      requestKeys: { current: new Set<string>() },
      completedAround: { current: new Set<string>() },
      targetIdentity: { current: null as string | null },
      scrollSucceededTarget: { current: null as string | null },
      reassertionTimer: { current: null as number | null },
      reassertionAttempted: { current: new Set<string>() },
      mounted: { current: true },
      lifecycle: { current: { sessionId, messageId, target, isVisible } },
    }),
    [],
  );
  const effectiveSessionId = target?.sessionId ?? sessionId;
  const effectiveMessageId = target?.messageId ?? messageId;
  const targetToken = target?.token ?? 0;
  const targetHostPanelId = target?.hostPanelId ?? "pending";
  const effectiveTargetKey =
    effectiveSessionId && effectiveMessageId
      ? `${effectiveSessionId}\u0000${effectiveMessageId}\u0000${targetToken}\u0000${targetHostPanelId}`
      : null;
  refs.lifecycle.current = {
    sessionId: effectiveSessionId,
    messageId: effectiveMessageId,
    target,
    isVisible,
  };
  useEffect(() => {
    refs.mounted.current = true;
    return () => {
      refs.mounted.current = false;
      cancelTargetReassertion(refs.reassertionTimer);
    };
  }, [refs]);
  usePendingMessageScrollEffect({
    messageListRef,
    target,
    onConsumed,
    readinessKey,
    isInitialMessagesLoading,
    isVisible,
    effectiveSessionId,
    effectiveMessageId,
    effectiveTargetKey,
    targetBelongsToHost: !target || target.sessionId === sessionId,
    store,
    refs,
    setIsLoading,
  });
  return { isLoading };
}

/** Computes the render-item key the unread "New" divider should appear
 * immediately before: tracks the latest rendered message id for session read
 * tracking, then maps the resulting divider anchor onto the grouped items. */
function useUnreadDividerBeforeItemKey(
  sessionId: string | null,
  isVisible: boolean,
  groupedItems: Parameters<typeof lastRenderedMessageId>[0],
  isInitialMessagesLoading: boolean,
) {
  const latestMessageId = useMemo(() => lastRenderedMessageId(groupedItems), [groupedItems]);
  const dividerAnchor = useSessionReadTracking(
    sessionId,
    isVisible,
    latestMessageId,
    isInitialMessagesLoading,
  );
  return useMemo(
    () => findUnreadDividerItemId(groupedItems, dividerAnchor),
    [groupedItems, dividerAnchor],
  );
}

/** Floating session-search overlay over the transcript: the search bar plus
 * its hits list, with next/prev cycling through hits. Renders nothing while
 * the search is closed. */
function SessionSearchOverlay({
  search,
  agentLabel,
  agentName,
}: {
  search: ReturnType<typeof useSessionSearch>;
  agentLabel: string | null;
  agentName: string | null;
}) {
  const currentIdx = search.activeHitId
    ? search.hits.findIndex((h) => h.id === search.activeHitId)
    : -1;
  const total = search.hits.length;
  const handleNext = useCallback(() => {
    if (!total) return;
    const next = search.hits[(Math.max(currentIdx, -1) + 1) % total];
    if (next) search.setActiveHit(next.id);
  }, [search, currentIdx, total]);
  const handlePrev = useCallback(() => {
    if (!total) return;
    const prevIdx = (Math.max(currentIdx, 0) - 1 + total) % total;
    const prev = search.hits[prevIdx];
    if (prev) search.setActiveHit(prev.id);
  }, [search, currentIdx, total]);
  if (!search.isOpen) return null;
  return (
    <div className="absolute top-2 right-2 z-20 flex flex-col items-end gap-1">
      <PanelSearchBar
        className="static"
        value={search.query}
        onChange={search.setQuery}
        onNext={handleNext}
        onPrev={handlePrev}
        onClose={search.close}
        matchInfo={{ current: currentIdx >= 0 ? currentIdx + 1 : 0, total }}
        isLoading={search.isSearching}
        // Session search already debounces in useDebouncedSearch; skip the
        // bar's debounce so we don't stack 150ms + 180ms per keystroke.
        debounceMs={0}
      />
      <SessionSearchHits
        hits={search.hits}
        query={search.query}
        activeHitId={search.activeHitId}
        onSelect={search.setActiveHit}
        isSearching={search.isSearching}
        agentLabel={agentLabel}
        agentName={agentName}
      />
    </div>
  );
}

/** Returns the AgentProfileOption for the session's profile, or null. Uses
 * primitive profile id to avoid getSnapshot-cache errors from returning
 * fresh objects on every selector call. */
function useSessionAgentProfile(sessionId: string | null | undefined) {
  const profileId = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId]?.agent_profile_id ?? null) : null,
  );
  return useAppStore((state) =>
    profileId
      ? (state.agentProfiles.items.find((p: { id: string }) => p.id === profileId) ?? null)
      : null,
  );
}

/** Resolves the agent profile name + registry slug for the given session.
 * Label is "Profile Name" from the "Agent • Profile Name" store label; slug
 * feeds <AgentLogo> which fetches the logo by agent type. */
function useSessionAgentIdentity(sessionId: string | null | undefined): {
  label: string | null;
  name: string | null;
} {
  const profile = useSessionAgentProfile(sessionId);
  // User-supplied session name wins over the derived profile label,
  // matching the session tab title precedence (resolveSessionTabTitle).
  const sessionName = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId]?.name ?? null) : null,
  );
  return useMemo(() => {
    if (!profile) return { label: sessionName, name: null };
    const parts = profile.label.split(" \u2022 ");
    const label = sessionName || parts[1] || parts[0] || profile.label;
    return { label, name: profile.agent_name ?? null };
  }, [profile, sessionName]);
}

type TaskChatPanelProps = {
  onSend?: (payload: ChatSubmitPayload) => ChatSubmitResult;
  sessionId?: string | null;
  taskId?: string | null;
  /**
   * Task this panel belongs to, independent of whether it has a session yet.
   * Only the status row uses it, so a task with no session still shows its
   * dependency and autopilot chips. `taskId` above stays session-gated because
   * it also drives plan mode, the composer, and read tracking.
   */
  statusTaskId?: string | null;
  onOpenFile?: (path: string, repo?: string) => void;
  showRequestChangesTooltip?: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  /** Callback to open a file at a specific line (for comment clicks) */
  onOpenFileAtLine?: (filePath: string) => void;
  /** Hide the sessions dropdown (session tabs in dockview replace it) */
  hideSessionsDropdown?: boolean;
  /**
   * Embedded multi-panel hosts do not own the global workbench or shortcuts.
   * They keep the conversation and composer, but suppress those side effects.
   */
  embedded?: boolean;
  /**
   * Whether this panel is the one actually on screen — gates the
   * Slack-style unread-divider read tracking (see
   * chat/use-session-read-tracking.ts). Dockview-hosted callers must pass
   * real tab-activation state (see hooks/use-panel-active.ts); other hosts
   * (mobile, quick chat, kanban preview) default to true since mounting
   * already implies visibility for them.
   */
  isVisible?: boolean;
  panelId?: string | null;
  /** Legacy message-only target for non-Dockview hosts. */
  pendingScrollToMessageId?: string | null;
  /** Identity-bearing target for mobile/non-Dockview prompt navigation. */
  pendingScrollTarget?: PendingMessageScrollTarget | null;
  /** Called after a non-Dockview scroll target reaches a rendered row. */
  onPendingScrollConsumed?: (messageId: string) => void;
};

type ScrollTargetConsumptionParams = {
  resolvedSessionId: string | null;
  isVisible: boolean;
  panelId: string | null;
  messageListRef: RefObject<MessageListHandle | null>;
  /** Whether the target row is present in the current message snapshot. */
  targetRendered?: boolean;
  isInitialMessagesLoading: boolean;
  /** Rendered-transcript revision: retry a retained target when a row mounts
   * after the initial load or when older pages are prepended. */
  renderedMessageCount: number;
};

function shouldDeferTargetWindowLoad(
  messages: readonly { id: string }[],
  targetMessageId: string,
  isInitialMessagesLoading: boolean,
  targetRendered: boolean,
) {
  if (isInitialMessagesLoading) return true;
  return messages.some((message) => message.id === targetMessageId) && targetRendered;
}

const pendingDockviewOwnerCleanups = new Map<string, number>();

function cancelDockviewOwnerCleanup(ownerKey: string) {
  const timer = pendingDockviewOwnerCleanups.get(ownerKey);
  if (timer === undefined) return;
  window.clearTimeout(timer);
  pendingDockviewOwnerCleanups.delete(ownerKey);
}

function scheduleDockviewOwnerCleanup(
  ownerKey: string,
  targetToken: number,
  clearScrollTarget: (token: number) => void,
) {
  cancelDockviewOwnerCleanup(ownerKey);
  const timer = window.setTimeout(() => {
    if (pendingDockviewOwnerCleanups.get(ownerKey) !== timer) return;
    pendingDockviewOwnerCleanups.delete(ownerKey);
    clearScrollTarget(targetToken);
  }, 0);
  pendingDockviewOwnerCleanups.set(ownerKey, timer);
}

type DockviewOwnerLifecycleOptions = {
  resolvedSessionId: string | null;
  panelId: string | null;
  scrollTarget: TranscriptScrollTarget | null;
  clearScrollTarget: (token: number) => void;
  clearScrollTargetForOwner: (sessionId: string, panelId: string) => void;
  cancelReassertion: () => void;
};

function useDockviewOwnerLifecycle({
  resolvedSessionId,
  panelId,
  scrollTarget,
  clearScrollTarget,
  clearScrollTargetForOwner,
  cancelReassertion,
}: DockviewOwnerLifecycleOptions) {
  const previousOwnerRef = useRef<{ sessionId: string; panelId: string } | null>(null);
  const ownerTargetRef = useRef<TranscriptScrollTarget | null>(null);
  ownerTargetRef.current =
    panelId && scrollTarget?.sessionId === resolvedSessionId && scrollTarget.hostPanelId === panelId
      ? scrollTarget
      : null;
  const ownerKey = ownerTargetRef.current
    ? `${ownerTargetRef.current.sessionId}\u0000${ownerTargetRef.current.hostPanelId}\u0000${ownerTargetRef.current.token}`
    : null;

  useEffect(() => {
    if (!panelId) return;
    const previousOwner = previousOwnerRef.current;
    previousOwnerRef.current = resolvedSessionId ? { sessionId: resolvedSessionId, panelId } : null;
    if (
      previousOwner &&
      (previousOwner.sessionId !== resolvedSessionId || previousOwner.panelId !== panelId)
    ) {
      clearScrollTargetForOwner(previousOwner.sessionId, previousOwner.panelId);
      cancelReassertion();
    }
  }, [cancelReassertion, clearScrollTargetForOwner, panelId, resolvedSessionId]);

  useEffect(() => {
    if (ownerKey) cancelDockviewOwnerCleanup(ownerKey);
  }, [ownerKey]);

  useEffect(() => {
    if (!panelId) return;
    return () => {
      cancelReassertion();
      const target = ownerTargetRef.current;
      if (!target || target.sessionId !== resolvedSessionId || target.hostPanelId !== panelId)
        return;
      const cleanupKey = `${target.sessionId}\u0000${target.hostPanelId}\u0000${target.token}`;
      scheduleDockviewOwnerCleanup(cleanupKey, target.token, clearScrollTarget);
    };
  }, [cancelReassertion, clearScrollTarget, panelId, resolvedSessionId]);
}

type DockviewTargetRefs = {
  activeTargetKey: MutableRefObject<string | null>;
  reassertionAttempted: MutableRefObject<Set<string>>;
  requestKeys: MutableRefObject<Set<string>>;
  completedAround: MutableRefObject<Set<string>>;
  scrollSucceededTarget: MutableRefObject<string | null>;
  reassertionTimer: MutableRefObject<number | null>;
  mounted: MutableRefObject<boolean>;
  lifecycle: MutableRefObject<{
    resolvedSessionId: string | null;
    isVisible: boolean;
    panelId: string | null;
  }>;
};

type DockviewTargetAttemptOptions = {
  target: TranscriptScrollTarget;
  targetKey: string;
  messageListRef: RefObject<MessageListHandle | null>;
  isInitialMessagesLoading: boolean;
  targetRendered: boolean;
  appStore: StoreApi<AppState>;
  refs: DockviewTargetRefs;
  setJumpLoading: (loading: boolean) => void;
  clearScrollTarget: (token: number) => void;
  isCurrentTarget: () => boolean;
};

function attemptDockviewScrollTarget(options: DockviewTargetAttemptOptions) {
  const {
    target,
    targetKey,
    messageListRef,
    isInitialMessagesLoading,
    targetRendered,
    appStore,
    refs,
    setJumpLoading,
    clearScrollTarget,
    isCurrentTarget,
  } = options;
  if (
    refs.completedAround.current.has(targetKey) &&
    (refs.reassertionTimer.current !== null || refs.reassertionAttempted.current.has(targetKey))
  ) {
    return;
  }
  if (!isCurrentTarget()) return;
  const didScroll = messageListRef.current?.scrollToMessage(target.messageId, {
    align: "start",
  });
  const consume = () => clearScrollTarget(target.token);
  const scheduleReassertion = () =>
    scheduleTargetReassertion({
      targetKey,
      timerRef: refs.reassertionTimer,
      attemptedRef: refs.reassertionAttempted,
      completedAroundRef: refs.completedAround,
      isCurrentTarget,
      scroll: () => {
        messageListRef.current?.scrollToMessage(target.messageId, {
          align: "start",
          behavior: "auto",
        });
      },
      consume,
    });
  if (didScroll) {
    refs.scrollSucceededTarget.current = targetKey;
    if (refs.completedAround.current.has(targetKey)) scheduleReassertion();
    else if (!refs.requestKeys.current.has(targetKey)) consume();
    return;
  }
  const sessionMessages = appStore.getState().messages.bySession[target.sessionId] ?? [];
  if (
    shouldDeferTargetWindowLoad(
      sessionMessages,
      target.messageId,
      isInitialMessagesLoading,
      targetRendered,
    ) ||
    refs.requestKeys.current.has(targetKey) ||
    refs.completedAround.current.has(targetKey)
  ) {
    return;
  }
  requestMessageWindowAround({
    sessionId: target.sessionId,
    messageId: target.messageId,
    targetKey,
    store: appStore,
    requestKeysRef: refs.requestKeys,
    mountedRef: refs.mounted,
    isCurrentTarget,
    setLoading: setJumpLoading,
    onMerged: () => {
      refs.completedAround.current.add(targetKey);
      if (refs.scrollSucceededTarget.current === targetKey) scheduleReassertion();
    },
    onCancelled: consume,
  });
}

type DockviewTargetEffectOptions = ScrollTargetConsumptionParams & {
  scrollTarget: TranscriptScrollTarget | null;
  appStore: StoreApi<AppState>;
  refs: DockviewTargetRefs;
  setJumpLoading: (loading: boolean) => void;
  clearScrollTarget: (token: number) => void;
  cancelReassertion: () => void;
};

function useDockviewTargetEffect(options: DockviewTargetEffectOptions) {
  const {
    resolvedSessionId,
    isVisible,
    panelId,
    messageListRef,
    targetRendered = false,
    isInitialMessagesLoading,
    renderedMessageCount,
    scrollTarget,
    appStore,
    refs,
    setJumpLoading,
    clearScrollTarget,
    cancelReassertion,
  } = options;
  useEffect(() => {
    if (
      !isVisible ||
      !scrollTarget ||
      !panelId ||
      scrollTarget.sessionId !== resolvedSessionId ||
      scrollTarget.hostPanelId !== panelId
    ) {
      return;
    }
    const targetKey = `${scrollTarget.sessionId}\u0000${scrollTarget.hostPanelId}\u0000${scrollTarget.token}`;
    if (refs.activeTargetKey.current !== targetKey) {
      refs.activeTargetKey.current = targetKey;
      refs.reassertionAttempted.current.delete(targetKey);
      refs.scrollSucceededTarget.current = null;
      cancelReassertion();
    }
    const isCurrentTarget = () => {
      const latest = useDockviewStore.getState().scrollTarget;
      return (
        refs.mounted.current &&
        refs.lifecycle.current.isVisible &&
        refs.lifecycle.current.resolvedSessionId === scrollTarget.sessionId &&
        refs.lifecycle.current.panelId === scrollTarget.hostPanelId &&
        latest?.token === scrollTarget.token &&
        latest.sessionId === scrollTarget.sessionId &&
        latest.hostPanelId === scrollTarget.hostPanelId
      );
    };
    let frame = requestAnimationFrame(() => {
      frame = requestAnimationFrame(() => {
        attemptDockviewScrollTarget({
          target: scrollTarget,
          targetKey,
          messageListRef,
          isInitialMessagesLoading,
          targetRendered,
          appStore,
          refs,
          setJumpLoading,
          clearScrollTarget,
          isCurrentTarget,
        });
      });
    });
    return () => cancelAnimationFrame(frame);
  }, [
    appStore,
    cancelReassertion,
    clearScrollTarget,
    isInitialMessagesLoading,
    isVisible,
    messageListRef,
    panelId,
    refs,
    renderedMessageCount,
    resolvedSessionId,
    scrollTarget,
    setJumpLoading,
    targetRendered,
  ]);
}

/**
 * Consumes Dockview prompt-history targets. Around-window targets stay owned
 * through their first rendered placement and one delayed reassertion.
 */
export function useScrollTargetConsumption({
  resolvedSessionId,
  isVisible,
  panelId,
  messageListRef,
  isInitialMessagesLoading,
  targetRendered = false,
  renderedMessageCount,
}: ScrollTargetConsumptionParams) {
  const scrollTarget = useDockviewStore((state) => state.scrollTarget);
  const clearScrollTarget = useDockviewStore((state) => state.clearScrollTarget);
  const clearScrollTargetForOwner = useDockviewStore((state) => state.clearScrollTargetForOwner);
  const appStore = useAppStoreApi();
  const [jumpLoading, setJumpLoading] = useState(false);
  const refs = useMemo<DockviewTargetRefs>(
    () => ({
      activeTargetKey: { current: null },
      reassertionAttempted: { current: new Set<string>() },
      requestKeys: { current: new Set<string>() },
      completedAround: { current: new Set<string>() },
      scrollSucceededTarget: { current: null },
      reassertionTimer: { current: null },
      mounted: { current: true },
      lifecycle: { current: { resolvedSessionId, isVisible, panelId } },
    }),
    [],
  );
  refs.lifecycle.current = { resolvedSessionId, isVisible, panelId };
  const cancelReassertion = useCallback(
    () => cancelTargetReassertion(refs.reassertionTimer),
    [refs],
  );
  useEffect(() => {
    refs.mounted.current = true;
    return () => {
      refs.mounted.current = false;
      cancelReassertion();
    };
  }, [cancelReassertion, refs]);
  useDockviewOwnerLifecycle({
    resolvedSessionId,
    panelId,
    scrollTarget,
    clearScrollTarget,
    clearScrollTargetForOwner,
    cancelReassertion,
  });
  useEffect(() => {
    if (isVisible) return;
    cancelReassertion();
    if (
      scrollTarget &&
      panelId &&
      scrollTarget.sessionId === resolvedSessionId &&
      scrollTarget.hostPanelId === panelId
    ) {
      const targetKey = `${scrollTarget.sessionId}\u0000${scrollTarget.hostPanelId}\u0000${scrollTarget.token}`;
      refs.completedAround.current.delete(targetKey);
      if (refs.scrollSucceededTarget.current === targetKey) clearScrollTarget(scrollTarget.token);
    }
  }, [
    cancelReassertion,
    clearScrollTarget,
    isVisible,
    panelId,
    refs,
    resolvedSessionId,
    scrollTarget,
  ]);
  useDockviewTargetEffect({
    resolvedSessionId,
    isVisible,
    panelId,
    messageListRef,
    targetRendered,
    isInitialMessagesLoading,
    renderedMessageCount,
    scrollTarget,
    appStore,
    refs,
    setJumpLoading,
    clearScrollTarget,
    cancelReassertion,
  });
  return jumpLoading;
}

// eslint-disable-next-line complexity, max-lines-per-function -- composes many sub-panels; each concern already factored into its own hook
export const TaskChatPanel = memo(function TaskChatPanel({
  onSend,
  sessionId = null,
  taskId: taskIdHint = null,
  statusTaskId = null,
  onOpenFile,
  showRequestChangesTooltip = false,
  onRequestChangesTooltipDismiss,
  onOpenFileAtLine,
  hideSessionsDropdown,
  embedded = false,
  isVisible = true,
  panelId = null,
  pendingScrollToMessageId = null,
  pendingScrollTarget,
  onPendingScrollConsumed,
}: TaskChatPanelProps) {
  const isArchived = useIsTaskArchived();
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  const launchErrorContext = useTaskLaunchErrorContext();
  const launchStatusSummary = useTaskStatusSummary(
    launchErrorContext?.taskId,
    launchErrorContext?.statusSummary,
  );
  const { t } = useTranslation();
  useSettingsData(true);
  const panelState = useChatPanelState({
    sessionId,
    taskId: taskIdHint,
    disableWorkbenchEffects: embedded,
    onOpenFile,
    onOpenFileAtLine,
  });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, onSend);
  const {
    resolvedSessionId,
    session,
    taskId,
    isWorking,
    messagesLoading,
    historyRefreshPending,
    isInitialMessagesLoading,
    groupedItems,
    allMessages,
    footerActionMessages,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    agentMessageCount,
    pendingClarification,
    pendingClarificationGroup,
  } = panelState;
  const activeLaunchError = launchStatusSummary?.active_error;
  const launchErrorOwned = Boolean(
    isTypedTaskLaunchError(activeLaunchError) &&
    isTaskLaunchErrorOwnedBySession(activeLaunchError, resolvedSessionId),
  );
  const showAgentStartHint = useComposerAgentStartHint(
    resolvedSessionId,
    session?.state,
    allMessages,
    footerActionMessages,
  );
  const { handleCancelTurn } = useChatPanelHandlers(resolvedSessionId, chatInputRef, {
    enableFocusShortcut: !embedded,
  });
  const { clarificationKey, handleClarificationResolved } = useClarificationKey(agentMessageCount);

  const panelRef = useRef<HTMLDivElement>(null);
  const messageListRef = useRef<MessageListHandle>(null);
  const dividerBeforeItemKey = useUnreadDividerBeforeItemKey(
    resolvedSessionId,
    isVisible,
    groupedItems,
    isInitialMessagesLoading,
  );
  // Kanban previews intentionally pass `isVisible=false` so they do not
  // advance the read cursor, but their transcript is rendered in a visible
  // non-Dockview host. Keep read visibility separate from scroll geometry.
  const transcriptIsVisible = panelId === null || isVisible;
  const dockviewTargetMessageId = useDockviewStore(
    (state) => state.scrollTarget?.messageId ?? null,
  );
  const isDockviewJumpLoading = useScrollTargetConsumption({
    resolvedSessionId,
    isVisible,
    panelId,
    messageListRef,
    isInitialMessagesLoading,
    targetRendered: Boolean(
      dockviewTargetMessageId &&
      allMessages.some((message) => message.id === dockviewTargetMessageId),
    ),
    renderedMessageCount: allMessages.length,
  });
  const { isLoading: isPendingJumpLoading } = usePendingMessageScroll({
    messageListRef,
    sessionId: resolvedSessionId,
    messageId: pendingScrollToMessageId,
    target: pendingScrollTarget,
    onConsumed: onPendingScrollConsumed,
    readinessKey: `${allMessages.length}:${isInitialMessagesLoading}:${
      allMessages[0]?.id ?? ""
    }:${allMessages.at(-1)?.id ?? ""}`,
    isInitialMessagesLoading,
    isVisible,
  });
  const isJumpLoading = isDockviewJumpLoading || isPendingJumpLoading;
  const lastPromptMessageId = useMemo(() => getLastUserMessageId(allMessages), [allMessages]);
  const lastPromptMessage = useMemo(
    () =>
      lastPromptMessageId
        ? (allMessages.find((message) => message.id === lastPromptMessageId) ?? null)
        : null,
    [allMessages, lastPromptMessageId],
  );
  const [lastPromptEdge, setLastPromptEdge] = useState<LastPromptEdge>("visible");
  const showAnchoredPromptBar = useAppStore((state) => state.userSettings.showAnchoredPromptBar);
  const showScrollToLastPrompt = useAppStore((state) => state.userSettings.showScrollToLastPrompt);
  const showScrollToStart = useAppStore((state) => state.userSettings.showScrollToStart);
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  // The anchored bar is a desktop-only, fine-pointer affordance; coarse
  // pointers use the compact scroll control instead.
  const showAnchoredBar = isFinePointer && !isMobile && showAnchoredPromptBar;
  const [anchoredBarHeight, setAnchoredBarHeight] = useState(0);
  const { anchoredBarVisible, scrollButtonEligible, scrollDirection } =
    resolveLastPromptControls(lastPromptEdge);
  const showScrollButton =
    showScrollToLastPrompt && Boolean(lastPromptMessageId) && scrollButtonEligible;
  const scrollToLastPrompt = useCallback(() => {
    if (lastPromptMessageId) {
      messageListRef.current?.scrollToMessage(lastPromptMessageId, { align: "start" });
    }
  }, [lastPromptMessageId]);
  const firstMessageId = useMemo(() => getFirstUserMessageId(allMessages), [allMessages]);
  const [isFirstMessageHidden, setIsFirstMessageHidden] = useState(false);
  const showScrollToStartButton =
    showScrollToStart && Boolean(firstMessageId) && isFirstMessageHidden;
  const { loadMoreRaw, hasMore } = useLazyLoadMessages(resolvedSessionId);
  // A paginated session's `firstMessageId` only reflects the oldest message in
  // the currently loaded page while `hasMore` is true — jumping there directly
  // lands on a partial-page boundary, not the transcript's real start. Drain
  // older pages first so `firstMessageId` (derived from `allMessages` above,
  // which grows as pages prepend) has settled on the true first prompt by the
  // time the scroll fires.
  const [pendingScrollToStart, setPendingScrollToStart] = useState(false);
  useDrainOlderMessages(resolvedSessionId, pendingScrollToStart && hasMore);
  useEffect(() => {
    if (!pendingScrollToStart || hasMore) return;
    setPendingScrollToStart(false);
    if (firstMessageId) {
      messageListRef.current?.scrollToMessage(firstMessageId, { align: "start" });
    }
  }, [pendingScrollToStart, hasMore, firstMessageId]);
  const scrollToStart = useCallback(() => {
    if (hasMore) {
      setPendingScrollToStart(true);
      return;
    }
    if (firstMessageId) {
      messageListRef.current?.scrollToMessage(firstMessageId, { align: "start" });
    }
  }, [hasMore, firstMessageId]);
  // Search can target backend rows before the visible transcript boundary.
  const search = useSessionSearch(resolvedSessionId, loadMoreRaw);
  const { label: agentLabel, name: agentName } = useSessionAgentIdentity(resolvedSessionId);
  usePanelSearch({
    containerRef: panelRef,
    isOpen: search.isOpen,
    onOpen: search.open,
    onClose: search.close,
  });

  // The message list has no focus-capturing child (unlike TipTap/xterm in the
  // plan/terminal panels), so clicking a message leaves focus on <body>. Make
  // the panel root itself focusable and route non-interactive clicks to it so
  // Ctrl+F can detect focus within the session panel.
  const handlePanelMouseDown = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => routePanelMouseDown(e, panelRef),
    [],
  );

  return (
    <PanelRoot
      ref={panelRef}
      data-testid="session-chat"
      data-panel-kind="session"
      data-session-id={resolvedSessionId ?? undefined}
      tabIndex={-1}
      onMouseDown={handlePanelMouseDown}
      className="outline-none"
    >
      <PanelBody padding={false} className="relative">
        {launchErrorContext && (
          <TaskChatLaunchError
            taskId={launchErrorContext.taskId}
            workspaceId={launchErrorContext.workspaceId}
            statusSummary={launchStatusSummary}
            sessionId={resolvedSessionId}
            repositories={launchErrorContext.repositories}
          />
        )}
        <TaskMarkdownFileLinkProvider
          taskId={taskId}
          sessionId={resolvedSessionId}
          worktreePath={getSessionWorkspacePath(session)}
          onOpenFile={onOpenFile}
        >
          <MessageList
            ref={messageListRef}
            items={groupedItems}
            messages={allMessages}
            footerActionMessages={footerActionMessages}
            permissionsByToolCallId={permissionsByToolCallId}
            childrenByParentToolCallId={childrenByParentToolCallId}
            taskId={taskId ?? undefined}
            sessionId={resolvedSessionId}
            messagesLoading={messagesLoading}
            historyRefreshPending={historyRefreshPending}
            isWorking={isWorking}
            sessionState={session?.state}
            worktreePath={getSessionWorkspacePath(session)}
            onOpenFile={onOpenFile}
            dividerBeforeItemKey={dividerBeforeItemKey}
            lastPromptMessageId={lastPromptMessageId}
            onLastPromptEdgeChange={setLastPromptEdge}
            firstMessageId={firstMessageId}
            onFirstMessageHiddenChange={setIsFirstMessageHidden}
            anchoredBarHeight={showAnchoredBar && lastPromptMessage ? anchoredBarHeight : 0}
            isVisible={transcriptIsVisible}
            launchErrorOwned={launchErrorOwned}
            launchErrorStamp={launchErrorOwned ? activeLaunchError?.stamp : undefined}
            launchErrorOccurredAt={launchErrorOwned ? activeLaunchError?.occurred_at : undefined}
            stickyPromptBar={
              showAnchoredBar && lastPromptMessage ? (
                <AnchoredLastPromptBar
                  promptText={lastPromptMessage.content}
                  isVisible={anchoredBarVisible}
                  onScrollUp={scrollToLastPrompt}
                  showScrollToLastPrompt={showScrollToLastPrompt}
                  onHeightChange={setAnchoredBarHeight}
                />
              ) : undefined
            }
          />
        </TaskMarkdownFileLinkProvider>
        {isJumpLoading && (
          <div
            data-testid="transcript-jump-loading"
            role="status"
            aria-live="polite"
            className="absolute right-3 top-3 rounded-md bg-background px-2 py-1 text-xs text-muted-foreground shadow"
          >
            {t("task:loading")}
          </div>
        )}
        <SessionSearchOverlay search={search} agentLabel={agentLabel} agentName={agentName} />
      </PanelBody>
      {!isArchived && (
        <ClarificationPanelSection
          pending={Boolean(pendingClarification)}
          messages={pendingClarificationGroup}
          onResolved={handleClarificationResolved}
          shortcutScopeRef={panelRef}
          maxHeightVh={50}
        />
      )}
      <ChatFooter
        isArchived={isArchived}
        chatInputRef={chatInputRef}
        clarificationKey={clarificationKey}
        onClarificationResolved={handleClarificationResolved}
        handleSubmit={handleSubmit}
        handleCancelTurn={handleCancelTurn}
        showRequestChangesTooltip={showRequestChangesTooltip}
        onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
        panelState={panelState}
        isSending={isSending}
        hideSessionsDropdown={hideSessionsDropdown}
        hidePlanMode={embedded}
        showScrollToLastPrompt={showScrollButton}
        onScrollToLastPrompt={scrollToLastPrompt}
        lastPromptScrollDirection={scrollDirection}
        showScrollToStart={showScrollToStartButton}
        onScrollToStart={scrollToStart}
        statusTaskId={statusTaskId ?? taskIdHint}
        showAgentStartHint={showAgentStartHint}
        launchErrorOwned={launchErrorOwned}
      />
    </PanelRoot>
  );
});

type ChatFooterProps = {
  isArchived: boolean;
  chatInputRef: RefObject<
    import("@/components/task/chat/chat-input-container").ChatInputContainerHandle | null
  >;
  clarificationKey: number;
  onClarificationResolved: () => void;
  handleSubmit: ReturnType<typeof useSubmitHandler>["handleSubmit"];
  handleCancelTurn: () => Promise<void>;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  panelState: ReturnType<typeof useChatPanelState>;
  isSending: boolean;
  hideSessionsDropdown?: boolean;
  hidePlanMode?: boolean;
  showScrollToLastPrompt: boolean;
  onScrollToLastPrompt: () => void;
  lastPromptScrollDirection: "up" | "down";
  showScrollToStart: boolean;
  onScrollToStart: () => void;
  statusTaskId: string | null;
  /** Recovered-idle sessions render the composer hint (see ChatInputArea). */
  showAgentStartHint: boolean;
  launchErrorOwned: boolean;
};

/**
 * Composer footer: renders the chat input area (or the read-only archived
 * banner) and forwards the recovered-idle agent-start-hint visibility from
 * the panel down to the input.
 */
function ChatFooter({
  isArchived,
  chatInputRef,
  clarificationKey,
  onClarificationResolved,
  handleSubmit,
  handleCancelTurn,
  showRequestChangesTooltip,
  onRequestChangesTooltipDismiss,
  panelState,
  isSending,
  hideSessionsDropdown,
  hidePlanMode,
  showScrollToLastPrompt,
  onScrollToLastPrompt,
  lastPromptScrollDirection,
  showScrollToStart,
  onScrollToStart,
  statusTaskId,
  showAgentStartHint,
  launchErrorOwned,
}: ChatFooterProps) {
  const { t } = useTranslation();
  if (isArchived) {
    return (
      <div className="bg-muted/50 flex-shrink-0 px-4 py-3 text-center text-sm text-muted-foreground border-t">
        {t("task:thisTaskIsArchivedAndRead")}
      </div>
    );
  }
  return (
    <ChatInputArea
      chatInputRef={chatInputRef}
      clarificationKey={clarificationKey}
      onClarificationResolved={onClarificationResolved}
      handleSubmit={handleSubmit}
      handleCancelTurn={handleCancelTurn}
      showRequestChangesTooltip={showRequestChangesTooltip}
      onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
      panelState={panelState}
      isSending={isSending}
      hideSessionsDropdown={hideSessionsDropdown}
      hidePlanMode={hidePlanMode}
      showScrollToLastPrompt={showScrollToLastPrompt}
      onScrollToLastPrompt={onScrollToLastPrompt}
      lastPromptScrollDirection={lastPromptScrollDirection}
      showScrollToStart={showScrollToStart}
      onScrollToStart={onScrollToStart}
      statusTaskId={statusTaskId}
      showAgentStartHint={showAgentStartHint}
      launchErrorOwned={launchErrorOwned}
    />
  );
}
