import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  IconChevronDown,
  IconChevronUp,
  IconClock,
  IconHourglass,
  IconRobot,
} from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useSessionPrompts } from "@/hooks/domains/session/use-session-prompts";
import { useLazyLoadPrompts } from "@/hooks/use-lazy-load-prompts";
import { useLazyLoadSentinel } from "@/hooks/use-lazy-load-sentinel";
import { useSessionTurnsState } from "@/hooks/domains/session/use-session-turns";
import { useMessageFavorite } from "@/hooks/domains/session/use-message-favorite";
import { formatDateTime, formatRelativeCompact } from "@/lib/i18n/formats";
import { cn } from "@/lib/utils";
import {
  buildPromptHistoryEntries,
  formatPromptDuration,
  type PromptHistoryEntry,
} from "@/lib/prompt-history";
import { PanelRoot } from "./panel-primitives";

type PromptHistoryPanelContentProps = { onNavigateToPrompt?: (messageId: string) => void };

const SENTINEL_TEST_ID = "prompt-history-sentinel";
const LOADING_OLDER_TEST_ID = "prompt-history-loading-older";
const SCROLL_TEST_ID = "prompt-history-scroll";
/** Minimum-display window for the loading message: after a page settles, keep
 * the message mounted for this long so the sentinel's re-arm loop (next page
 * firing right after a positive settle) renders a continuous indicator instead
 * of a per-page flash. */
const LOADING_GRACE_MS = 400;

function PromptHistoryLoadError({
  rootRef,
  onRetry,
}: {
  rootRef: RefObject<HTMLDivElement | null>;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  return (
    <PanelRoot
      ref={rootRef}
      data-testid="prompt-history-panel"
      className="flex flex-col items-center gap-3 p-4 text-sm text-muted-foreground"
    >
      <span>{t("task:error")}</span>
      <Button
        data-testid="prompt-history-retry"
        className="min-h-11 md:min-h-0"
        onClick={onRetry}
        variant="outline"
      >
        {t("task:retry")}
      </Button>
    </PanelRoot>
  );
}

function PromptHistoryPassthrough({ rootRef }: { rootRef: RefObject<HTMLDivElement | null> }) {
  const { t } = useTranslation();
  return (
    <PanelRoot
      ref={rootRef}
      data-testid="prompt-history-panel"
      className="p-4 text-sm text-muted-foreground"
    >
      {t("task:promptHistoryEmpty")}
    </PanelRoot>
  );
}

/** The prompt-history panel: builds entries from the session's messages and
 * turns and renders one expandable row per prompt. Older pages auto-load via
 * the shared lazy-load sentinel (like the transcript's scroll-up sentinel),
 * with no load-more button. Passthrough sessions are an unconditional
 * empty-state with NO controls (rows, arrows, sentinel, loading indicators):
 * the transcript the arrow would jump to does not exist. */
export function PromptHistoryPanelContent({ onNavigateToPrompt }: PromptHistoryPanelContentProps) {
  const { t } = useTranslation();
  const rootRef = useRef<HTMLDivElement>(null);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const session = useAppStore((state) => (sessionId ? state.taskSessions.items[sessionId] : null));
  const {
    prompts,
    isLoading: messagesLoading,
    fetchFailed,
    retryPrompts,
  } = useSessionPrompts(sessionId);
  const { loadMore, hasMore, isLoadingMore } = useLazyLoadPrompts(sessionId);
  const { turns, isHydrated: turnsHydrated } = useSessionTurnsState(sessionId);
  const entries = useMemo(() => {
    const derived = buildPromptHistoryEntries(prompts, turns);
    if (turnsHydrated) return derived;
    return derived.map((entry) => ({ ...entry, durationSeconds: null }));
  }, [prompts, turns, turnsHydrated]);
  const [expanded, setExpanded] = useState<string | null>(null);
  const maxHeight = usePanelRowMaxHeight(rootRef);
  const contentRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  // Floating only when the prompt rows actually overflow the panel (the panel
  // scrolls); measured from the dedicated content wrapper against the inner
  // scroller's content box, so the loading indicator's own presence can never
  // flip the answer.
  const isScrollable = usePanelContentScrollable(scrollRef, contentRef);
  // Minimum-display grace so consecutive auto-loads show one continuous
  // indicator instead of a per-page flash (scoped to the active session).
  const showLoadingGrace = useLoadingGrace(sessionId, isLoadingMore);
  const shouldPaginate = hasMore && !entries.some((entry) => entry.promptNumber === 1);
  const showLoading = shouldPaginate && (isLoadingMore || showLoadingGrace);
  const { sentinelRef, onUserGesture } = usePanelOlderPromptSentinel({
    scrollRef,
    shouldPaginate,
    messagesLoading,
    isLoadingMore,
    loadMore,
  });

  if (session?.is_passthrough) return <PromptHistoryPassthrough rootRef={rootRef} />;

  if (fetchFailed && entries.length === 0) {
    return <PromptHistoryLoadError rootRef={rootRef} onRetry={retryPrompts} />;
  }
  if (entries.length === 0) {
    // Keep PanelRoot as the stable top-level element in EVERY branch (same
    // element type at the same position) so the root element and its
    // ResizeObserver survive the empty→rows transition; only the inner
    // content varies. An empty panel never scrolls, so the loading message
    // stays in-flow.
    const spec = emptyEntriesSpec({
      showLoading,
      messagesLoading,
      hasMore,
      sentinelRef,
      emptyText: t("task:promptHistoryEmpty"),
      loadingText: t("task:loading"),
      loadingOlderText: t("task:loadingOlderMessages"),
    });
    return (
      <PanelRoot
        ref={rootRef}
        data-testid="prompt-history-panel"
        className="relative overflow-hidden"
      >
        <PromptHistoryEmptyScroller
          spec={spec}
          scrollRef={scrollRef}
          onUserGesture={onUserGesture}
        />
      </PanelRoot>
    );
  }

  return (
    <PanelRoot
      ref={rootRef}
      data-testid="prompt-history-panel"
      className="relative overflow-hidden"
    >
      {/* Distinct scroller from the positioned outer root: an absolute child
       * of a scroll container moves with its content, so the floating chip
       * must anchor to the panel viewport. */}
      <PromptHistoryScroller
        scrollRef={scrollRef}
        interactive={shouldPaginate}
        onUserGesture={onUserGesture}
      >
        <PromptHistoryRowList
          contentRef={contentRef}
          entries={entries}
          sessionId={sessionId}
          expanded={expanded}
          maxHeight={maxHeight}
          onToggle={(messageId) => setExpanded(expanded === messageId ? null : messageId)}
          onNavigate={onNavigateToPrompt}
          shouldPaginate={shouldPaginate}
          sentinelRef={sentinelRef}
        />
        {showLoading && !isScrollable && (
          <LoadingMoreMessage floating={false} text={t("task:loadingOlderMessages")} />
        )}
      </PromptHistoryScroller>
      {showLoading && isScrollable && (
        <LoadingMoreMessage floating text={t("task:loadingOlderMessages")} />
      )}
    </PanelRoot>
  );
}

/** The panel's older-prompt auto-load sentinel configuration: bottom-margin
 * root, re-arm while intersecting, join in-flight requests, and stick to the
 * bottom while the user waits there so appended rows cannot stall the load. */
function usePanelOlderPromptSentinel(opts: {
  scrollRef: RefObject<HTMLDivElement | null>;
  shouldPaginate: boolean;
  messagesLoading: boolean;
  isLoadingMore: boolean;
  loadMore: () => Promise<number>;
}): {
  sentinelRef: (node: HTMLDivElement | null) => void;
  onUserGesture: () => void;
} {
  return useLazyLoadSentinel(
    opts.scrollRef,
    opts.shouldPaginate,
    opts.messagesLoading,
    opts.isLoadingMore,
    opts.loadMore,
    {
      rootMargin: "0px 0px 200px 0px",
      rearmWhileIntersecting: true,
      joinInFlightWhileLoading: true,
      // Keep loading while the user waits at the bottom: appended rows would
      // push the sentinel below the viewport and stall the auto-load.
      stickToBottomWhileLoading: true,
    },
  );
}

/** The zero-entries scroller wrapper: the same scroller structure as the rows
 * branch, rendered inside the panel's stable PanelRoot (which stays the
 * top-level element across branches so its ResizeObserver survives). */
function PromptHistoryEmptyScroller({
  spec,
  scrollRef,
  onUserGesture,
}: {
  spec: { content: React.ReactNode; interactive: boolean };
  scrollRef: RefObject<HTMLDivElement | null>;
  onUserGesture: () => void;
}) {
  return (
    <PromptHistoryScroller
      scrollRef={scrollRef}
      interactive={spec.interactive}
      onUserGesture={onUserGesture}
    >
      {spec.content}
    </PromptHistoryScroller>
  );
}

/** The prompt rows plus the pagination sentinel, inside the content wrapper
 * the panel measures scrollability from. The sentinel stays the final in-flow
 * child with a stable geometry, so the loading indicator can never reflow the
 * rows or shift it. */
function PromptHistoryRowList({
  contentRef,
  entries,
  sessionId,
  expanded,
  maxHeight,
  onToggle,
  onNavigate,
  shouldPaginate,
  sentinelRef,
}: {
  contentRef: RefObject<HTMLDivElement | null>;
  entries: PromptHistoryEntry[];
  sessionId: string | null;
  expanded: string | null;
  maxHeight: string;
  onToggle: (messageId: string) => void;
  onNavigate?: (messageId: string) => void;
  shouldPaginate: boolean;
  sentinelRef: (node: HTMLDivElement | null) => void;
}) {
  return (
    <div ref={contentRef} className="shrink-0">
      {entries.map((entry, index) => (
        <PromptHistoryRow
          key={entry.messageId}
          sessionId={sessionId}
          entry={entry}
          index={index}
          expanded={expanded === entry.messageId}
          maxHeight={maxHeight}
          onToggle={() => onToggle(entry.messageId)}
          onNavigate={onNavigate}
        />
      ))}
      {shouldPaginate && (
        <div ref={sentinelRef} data-testid={SENTINEL_TEST_ID} aria-hidden="true" />
      )}
    </div>
  );
}

/** The panel's inner scroll container. It is a distinct element from the
 * positioned outer root so the floating loading message can anchor to the
 * panel viewport instead of the scrolled content (an absolutely positioned
 * child of a scroll container moves with its content). */
function PromptHistoryScroller({
  scrollRef,
  interactive,
  onUserGesture,
  children,
}: {
  scrollRef: RefObject<HTMLDivElement | null>;
  interactive: boolean;
  onUserGesture: () => void;
  children: React.ReactNode;
}) {
  return (
    <div
      ref={scrollRef}
      data-testid={SCROLL_TEST_ID}
      className="h-full min-h-0 overflow-y-auto p-2"
      onWheel={interactive ? onUserGesture : undefined}
      // onTouchStart fires once per gesture; onTouchMove would retry on every
      // pixel of movement (30-120 events/sec) while a gesture drags.
      onTouchStart={interactive ? onUserGesture : undefined}
    >
      {children}
    </div>
  );
}

/** The "loading older messages" indicator. `floating` pins it to the panel
 * bottom, out of the content flow (full panel, so its presence cannot change
 * the scroll layout); `inline` renders it in the content flow directly under
 * the last message (short panel). */
function LoadingMoreMessage({ floating, text }: { floating: boolean; text: string }) {
  return floating ? (
    <div
      data-testid={LOADING_OLDER_TEST_ID}
      className="pointer-events-none absolute inset-x-0 bottom-2 z-10 flex justify-center"
    >
      <div className="rounded-full border border-border bg-card/95 px-3 py-1 text-xs text-muted-foreground shadow-sm">
        {text}
      </div>
    </div>
  ) : (
    <div
      data-testid={LOADING_OLDER_TEST_ID}
      className="py-2 text-center text-xs text-muted-foreground"
    >
      {text}
    </div>
  );
}

/** Empty-entry non-passthrough states: older-page loading takes precedence
 * over neutral initial loading, an unexhausted session stays paginatable with
 * the sentinel, and only the definitive empty state (no entries, no hasMore,
 * no loading) renders the empty text. Returns content plus the wrapper props
 * so the caller keeps a stable PanelRoot element across branch switches. */
function emptyEntriesSpec({
  messagesLoading,
  showLoading,
  hasMore,
  sentinelRef,
  emptyText,
  loadingText,
  loadingOlderText,
}: {
  messagesLoading: boolean;
  showLoading: boolean;
  hasMore: boolean;
  sentinelRef: (node: HTMLDivElement | null) => void;
  emptyText: string;
  loadingText: string;
  loadingOlderText: string;
}): { content: React.ReactNode; interactive: boolean } {
  if (showLoading) {
    // No entries means the panel has nothing to scroll: the loading message
    // sits in the content flow under the (empty) list, never floating.
    return {
      content: (
        <>
          <div ref={sentinelRef} data-testid={SENTINEL_TEST_ID} aria-hidden="true" />
          <LoadingMoreMessage floating={false} text={loadingOlderText} />
        </>
      ),
      interactive: true,
    };
  }
  if (messagesLoading) {
    return {
      content: <div className="p-4 text-sm text-muted-foreground">{loadingText}</div>,
      interactive: false,
    };
  }
  if (hasMore) {
    // No user prompts in the loaded page but older pages remain: keep the
    // panel paginatable so the sentinel discovers older prompts instead of
    // declaring the session empty.
    return {
      content: <div ref={sentinelRef} data-testid={SENTINEL_TEST_ID} aria-hidden="true" />,
      interactive: true,
    };
  }
  return {
    content: <div className="p-4 text-sm text-muted-foreground">{emptyText}</div>,
    interactive: false,
  };
}

/** Tracks the panel root's height via ResizeObserver and returns 40% of it as
 * a CSS max-height string (falling back to "40vh") for expanded prompt rows. */
function usePanelRowMaxHeight(rootRef: RefObject<HTMLDivElement | null>) {
  const [maxHeight, setMaxHeight] = useState<string>("40vh");
  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    /** Recomputes the row max-height from the current root height. */
    const updateHeight = () =>
      setMaxHeight(root.clientHeight ? `${Math.round(root.clientHeight * 0.4)}px` : "40vh");
    updateHeight();
    const observer = new ResizeObserver(updateHeight);
    observer.observe(root);
    return () => observer.disconnect();
  }, [rootRef]);
  return maxHeight;
}

/** True when `contentHeight` exceeds the root's scrollable (content-box)
 * height. `rootClientHeight` includes the root's vertical padding, which is
 * not part of the scrollable viewport, so it is subtracted. */
export function overflowsPanel(
  contentHeight: number,
  rootClientHeight: number,
  verticalPadding: number,
): boolean {
  return contentHeight > rootClientHeight - verticalPadding;
}

/** True when the prompt rows overflow the scroller, i.e. the panel actually
 * scrolls. Measured from a dedicated content wrapper (rows + sentinel) against
 * the inner scroller's content box so the loading indicator's own presence
 * never affects the answer. Re-measured after every commit (pages append, rows
 * expand/collapse) and whenever the scroller resizes externally (e.g. a
 * dockview width-only drag commits no React render). The observer is recreated
 * per commit so it also attaches when the scroller first appears (the
 * passthrough and empty branches do not render it). */
function usePanelContentScrollable(
  scrollRef: RefObject<HTMLDivElement | null>,
  contentRef: RefObject<HTMLDivElement | null>,
): boolean {
  const [isScrollable, setIsScrollable] = useState(false);
  const measure = useCallback(() => {
    const scroller = scrollRef.current;
    const content = contentRef.current;
    if (!scroller || !content) return false;
    const style = getComputedStyle(scroller);
    const verticalPadding =
      (parseFloat(style.paddingTop) || 0) + (parseFloat(style.paddingBottom) || 0);
    return overflowsPanel(content.scrollHeight, scroller.clientHeight, verticalPadding);
  }, [scrollRef, contentRef]);
  useLayoutEffect(() => {
    setIsScrollable(measure());
    const scroller = scrollRef.current;
    if (!scroller) return;
    const observer = new ResizeObserver(() => setIsScrollable(measure()));
    observer.observe(scroller);
    return () => observer.disconnect();
  });
  return isScrollable;
}

/** Minimum-display grace for the loading message: once a load starts, keep the
 * message mounted for LOADING_GRACE_MS after the request settles so the
 * sentinel's re-arm loop (next page firing right after a positive settle)
 * renders a continuous indicator instead of a per-page flash.
 *
 * The grace is bound to the session that produced it. The returned value is
 * derived from the state's own session identity, so it is `false` for a
 * session that did not start its own load synchronously during the render in
 * which the active session changes (the effect only confirms/clears
 * afterwards, so the new session can never paint the old session's grace), and
 * switching away and back within the window never revives an old session's
 * grace. */
function useLoadingGrace(sessionId: string | null, isLoadingMore: boolean): boolean {
  const [grace, setGrace] = useState<{ sessionId: string | null; show: boolean }>({
    sessionId,
    show: false,
  });
  useEffect(() => {
    if (grace.sessionId !== sessionId) {
      // The active session changed: drop the previous session's grace, but
      // carry over an already in-flight load of the new session (the shared
      // per-session flag can be true from a transcript-initiated request), so
      // its settle still gets the minimum-display grace.
      setGrace({ sessionId, show: isLoadingMore });
      return;
    }
    if (isLoadingMore) {
      setGrace({ sessionId, show: true });
      return;
    }
    const timer = window.setTimeout(() => setGrace({ sessionId, show: false }), LOADING_GRACE_MS);
    return () => window.clearTimeout(timer);
  }, [sessionId, isLoadingMore, grace.sessionId]);
  return grace.sessionId === sessionId && grace.show;
}

/** The small `#N` ordinal label at the start of a prompt bubble. Rendered
 * only when the entry carries a known ordinal; `#N` is not translatable copy
 * (precedent: `#${pr.pr_number}`). */
function PromptNumberLabel({ index, promptNumber }: { index: number; promptNumber: number }) {
  return (
    <span
      data-testid={`prompt-history-number-${index}`}
      aria-hidden="true"
      className="mr-1 shrink-0 text-[10px] font-medium leading-4 text-muted-foreground"
    >
      #{promptNumber}
    </span>
  );
}

type PromptHistoryRowProps = {
  sessionId: string | null;
  entry: PromptHistoryEntry;
  index: number;
  expanded: boolean;
  maxHeight: string;
  onToggle: () => void;
  onNavigate?: (messageId: string) => void;
};

/** One prompt-history row: the prompt text (truncated, or expanded into a
 * scrollable box) inside the transcript-style bubble, its relative time and
 * duration. The bubble is directly clickable for transcript navigation; the
 * expand/collapse chevron floats over the truncated text's ellipsis when it
 * overflows. */
function PromptHistoryRow({
  sessionId,
  entry,
  index,
  expanded,
  maxHeight,
  onToggle,
  onNavigate,
}: PromptHistoryRowProps) {
  const { t } = useTranslation();
  const { isFavorite } = useMessageFavorite(sessionId ?? "", entry.messageId);
  const textRef = useRef<HTMLSpanElement>(null);
  const [overflow, setOverflow] = useState(false);
  useEffect(() => {
    const text = textRef.current;
    if (!text) return;
    /** Recomputes whether the prompt text overflows its single-line span. */
    const update = () => setOverflow(text.scrollWidth > text.clientWidth);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(text);
    return () => observer.disconnect();
  }, []);
  const showToggle = overflow || expanded;
  return (
    <div data-testid={`prompt-history-row-${index}`} className="flex items-start gap-1 py-1">
      <div className="min-w-0 flex-1">
        {/* Same bubble as the transcript's user message: markdown-body
            font, rounded-2xl, blue when not favorited / yellow when the
            message is starred — with lighter padding. The prompt bubble is
            directly clickable for transcript navigation. */}
        <div
          role="button"
          tabIndex={0}
          data-message-id={entry.messageId}
          className={cn(
            "markdown-body markdown-body-user group relative flex min-h-11 cursor-pointer items-center overflow-hidden rounded-2xl px-3 py-1.5 md:min-h-0",
            isFavorite ? "bg-yellow-200/50 dark:bg-yellow-500/10" : "bg-primary/30",
          )}
          onClick={(event) => {
            if (event.target instanceof HTMLElement && event.target.closest("button")) return;
            onNavigate?.(entry.messageId);
          }}
          onKeyDown={(event) => {
            if (event.key !== "Enter" && event.key !== " ") return;
            if (event.target instanceof HTMLElement && event.target.closest("button")) return;
            event.preventDefault();
            onNavigate?.(entry.messageId);
          }}
        >
          {entry.promptNumber !== null && (
            <PromptNumberLabel index={index} promptNumber={entry.promptNumber} />
          )}
          {entry.isAgentPrompt && (
            <IconRobot
              className="mr-1 inline-block h-3.5 w-3.5 align-text-bottom"
              aria-hidden="true"
            />
          )}
          <span ref={textRef} className={expanded ? "hidden" : "min-w-0 flex-1 truncate"}>
            {entry.content}
          </span>
          {expanded && (
            <div
              data-testid={`prompt-history-expanded-box-${index}`}
              className="min-w-0 flex-1 overflow-y-auto whitespace-normal"
              style={{ maxHeight }}
            >
              {entry.content}
            </div>
          )}
          {showToggle && (
            <Button
              variant="ghost"
              size="icon"
              className={cn(
                "absolute right-1 z-10 size-11 cursor-pointer rounded-md bg-background/70 opacity-0 transition-opacity hover:bg-background/90 group-hover:opacity-100 focus-visible:opacity-100 sm:size-6",
                expanded ? "top-1" : "top-1/2 -translate-y-1/2",
              )}
              aria-expanded={expanded}
              aria-label={t(expanded ? "task:collapsePrompt" : "task:expandPrompt")}
              data-testid={`prompt-history-expand-${index}`}
              onClick={onToggle}
            >
              {expanded ? <IconChevronUp size={14} /> : <IconChevronDown size={14} />}
            </Button>
          )}
        </div>
      </div>
      <div className="flex shrink-0 flex-col items-end gap-0.5 text-xs leading-tight text-muted-foreground">
        <time
          dateTime={entry.sentAt}
          title={formatDateTime(entry.sentAt)}
          className="inline-flex items-center gap-1"
        >
          <IconClock className="h-3 w-3 shrink-0" aria-hidden="true" />
          {formatRelativeCompact(entry.sentAt)}
        </time>
        <PromptDuration durationSeconds={entry.durationSeconds} index={index} />
      </div>
    </div>
  );
}

/** Renders the formatted duration label for a prompt row, or null when the
 * entry has no recorded duration. The hourglass marks it as elapsed time
 * and matches the surrounding text size. */
function PromptDuration({
  durationSeconds,
  index,
}: {
  durationSeconds: number | null;
  index: number;
}) {
  const { t } = useTranslation();
  if (durationSeconds === null) return null;
  return (
    <span
      data-testid={`prompt-history-duration-${index}`}
      className="inline-flex items-center gap-1"
    >
      <IconHourglass className="h-3 w-3 shrink-0" aria-hidden="true" />
      {formatPromptDuration(durationSeconds, {
        s: t("task:durationUnitSeconds"),
        m: t("task:durationUnitMinutes"),
        h: t("task:durationUnitHours"),
      })}
    </span>
  );
}
