import { useEffect, useMemo, useRef, useState, type RefObject } from "react";
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
import { useSessionMessages } from "@/hooks/domains/session/use-session-messages";
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

/** The prompt-history panel: builds entries from the session's messages and
 * turns and renders one expandable row per prompt; shows an empty-state
 * message for passthrough sessions or when there are no entries. */
export function PromptHistoryPanelContent({ onNavigateToPrompt }: PromptHistoryPanelContentProps) {
  const { t } = useTranslation();
  const rootRef = useRef<HTMLDivElement>(null);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const session = useAppStore((state) => (sessionId ? state.taskSessions.items[sessionId] : null));
  const { messages } = useSessionMessages(sessionId);
  const { turns, isHydrated: turnsHydrated } = useSessionTurnsState(sessionId);
  const entries = useMemo(() => {
    const derived = buildPromptHistoryEntries(messages, turns);
    if (turnsHydrated) return derived;
    return derived.map((entry) => ({ ...entry, durationSeconds: null }));
  }, [messages, turns, turnsHydrated]);
  const [expanded, setExpanded] = useState<string | null>(null);
  const maxHeight = usePanelRowMaxHeight(rootRef);

  if (session?.is_passthrough || entries.length === 0) {
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

  return (
    <PanelRoot ref={rootRef} data-testid="prompt-history-panel" className="overflow-y-auto p-2">
      {entries.map((entry, index) => (
        <PromptHistoryRow
          key={entry.messageId}
          sessionId={sessionId}
          entry={entry}
          index={index}
          expanded={expanded === entry.messageId}
          maxHeight={maxHeight}
          onToggle={() => setExpanded(expanded === entry.messageId ? null : entry.messageId)}
          onNavigate={onNavigateToPrompt}
        />
      ))}
    </PanelRoot>
  );
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
