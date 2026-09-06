import { useLayoutEffect, useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  IconChevronDown,
  IconChevronUp,
  IconClock,
  IconHourglass,
  IconRobot,
} from "@tabler/icons-react";
import { useMessageFavorite } from "@/hooks/domains/session/use-message-favorite";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { formatDateTime, formatRelativeCompact } from "@/lib/i18n/formats";
import { formatPromptDuration, type PromptHistoryEntry } from "@/lib/prompt-history";
import { cn } from "@/lib/utils";
import { PromptMentionText } from "./chat/messages/prompt-mention-components";

export type PromptHistoryRowProps = {
  sessionId: string | null;
  entry: PromptHistoryEntry;
  index: number;
  promptNames: string[];
  expanded: boolean;
  maxHeight: string;
  onToggle: () => void;
  onNavigate?: (messageId: string) => void;
};

function isNestedInteractiveTarget(target: EventTarget | null, currentTarget: Element) {
  const interactiveTarget =
    target instanceof Element ? target.closest('button,[role="button"]') : null;
  return interactiveTarget !== null && interactiveTarget !== currentTarget;
}

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

function usePromptHistoryOverflow(
  textRef: RefObject<HTMLSpanElement | null>,
  content: string,
  promptNames: PromptHistoryRowProps["promptNames"],
  expanded: boolean,
) {
  const [overflow, setOverflow] = useState(false);
  useLayoutEffect(() => {
    const text = textRef.current;
    if (!text) return;
    const update = () => setOverflow(text.scrollWidth > text.clientWidth);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(text);
    return () => observer.disconnect();
  }, [textRef, content, promptNames, expanded]);
  return overflow;
}

/** One prompt-history row with transcript navigation and prompt expansion. */
export function PromptHistoryRow({
  sessionId,
  entry,
  index,
  promptNames,
  expanded,
  maxHeight,
  onToggle,
  onNavigate,
}: PromptHistoryRowProps) {
  const { t } = useTranslation();
  const rowLabelId = `prompt-history-label-${entry.messageId}`;
  const rowLabel =
    entry.promptNumber == null
      ? t("task:promptHistoryPromptLabelGeneric")
      : t("task:promptHistoryPromptLabel", { number: entry.promptNumber });
  return (
    <div data-testid={`prompt-history-row-${index}`} className="flex items-start gap-1 py-1">
      <div id={rowLabelId} className="sr-only">
        {rowLabel}
      </div>
      <PromptHistoryBubble
        sessionId={sessionId}
        entry={entry}
        index={index}
        promptNames={promptNames}
        expanded={expanded}
        maxHeight={maxHeight}
        onToggle={onToggle}
        onNavigate={onNavigate}
        rowLabelId={rowLabelId}
        rowLabel={rowLabel}
      />
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

function PromptHistoryExpandButton({
  index,
  expanded,
  onToggle,
  labelId,
}: {
  index: number;
  expanded: boolean;
  onToggle: () => void;
  labelId: string;
}) {
  const { isFinePointer } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  return (
    <Button
      variant="ghost"
      size="icon"
      className={cn(
        "absolute right-1 z-10 cursor-pointer rounded-md bg-background/70 transition-opacity hover:bg-background/90",
        isFinePointer
          ? "size-6 opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
          : "size-11 opacity-100",
        expanded ? "top-1" : "top-1/2 -translate-y-1/2",
      )}
      aria-expanded={expanded}
      aria-describedby={labelId}
      aria-label={t(expanded ? "task:collapsePrompt" : "task:expandPrompt")}
      data-testid={`prompt-history-expand-${index}`}
      onClick={(event) => {
        event.stopPropagation();
        onToggle();
      }}
    >
      {expanded ? <IconChevronUp size={14} /> : <IconChevronDown size={14} />}
    </Button>
  );
}

function PromptHistoryBubble({
  sessionId,
  entry,
  index,
  promptNames,
  expanded,
  maxHeight,
  onToggle,
  onNavigate,
  rowLabelId,
  rowLabel,
}: PromptHistoryRowProps & { rowLabelId: string; rowLabel: string }) {
  const { isFavorite } = useMessageFavorite(sessionId ?? "", entry.messageId);
  const textRef = useRef<HTMLSpanElement>(null);
  const overflow = usePromptHistoryOverflow(textRef, entry.content, promptNames, expanded);
  const showToggle = overflow || expanded;
  return (
    <div className="relative min-w-0 flex-1">
      <div
        data-message-id={entry.messageId}
        aria-describedby={onNavigate ? rowLabelId : undefined}
        className={cn(
          "markdown-body markdown-body-user group relative flex min-h-11 cursor-pointer items-center overflow-hidden rounded-2xl px-3 py-1.5 md:min-h-0",
          isFavorite ? "bg-yellow-200/50 dark:bg-yellow-500/10" : "bg-primary/30",
        )}
        onClick={(event) => {
          if (isNestedInteractiveTarget(event.target, event.currentTarget)) return;
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
          <PromptMentionText
            text={entry.content}
            promptNames={promptNames}
            keyPrefix={`history-${entry.messageId}`}
          />
        </span>
        {expanded && (
          <div
            data-testid={`prompt-history-expanded-box-${index}`}
            className="min-w-0 flex-1 overflow-y-auto whitespace-normal"
            style={{ maxHeight }}
          >
            <PromptMentionText
              text={entry.content}
              promptNames={promptNames}
              keyPrefix={`history-expanded-${entry.messageId}`}
            />
          </div>
        )}
        {showToggle && (
          <PromptHistoryExpandButton
            index={index}
            expanded={expanded}
            onToggle={onToggle}
            labelId={rowLabelId}
          />
        )}
      </div>
      {onNavigate && (
        <button
          type="button"
          data-testid={`prompt-history-navigate-${index}`}
          aria-label={rowLabel}
          aria-describedby={rowLabelId}
          className="pointer-events-none absolute inset-0 z-0 min-h-11 w-full rounded-2xl opacity-0 outline-none focus-visible:pointer-events-auto focus-visible:z-20 focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-ring"
          onClick={() => onNavigate(entry.messageId)}
          onKeyDown={(event) => {
            if (event.key !== "Enter" && event.key !== " ") return;
            event.preventDefault();
            onNavigate(entry.messageId);
          }}
        />
      )}
    </div>
  );
}

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
