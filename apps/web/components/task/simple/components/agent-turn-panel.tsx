"use client";

import { useMemo, useState } from "react";
import { IconChevronDown, IconLoader2 } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { useSessionMessages } from "@/hooks/domains/session/use-session-messages";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import type { Message } from "@/lib/types/http";

type AgentTurnPanelProps = {
  taskId: string;
  sessionId: string;
  /** Exclusive lower bound on message created_at (the previous turn boundary). */
  fromExclusive: string | null;
  /** Inclusive upper bound on message created_at (this turn's bridging comment). */
  toInclusive: string;
  /** When true, renders a small spinner header instead of the closed-turn duration text. */
  isLive?: boolean;
};

function formatDuration(ms: number): string {
  if (ms < 1000) return "<1s";
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remaining = seconds % 60;
  return remaining > 0 ? `${minutes}m ${remaining}s` : `${minutes}m`;
}

function turnDuration(messages: Message[]): string | null {
  if (messages.length === 0) return null;
  const first = new Date(messages[0].created_at).getTime();
  const last = new Date(messages[messages.length - 1].created_at).getTime();
  if (isNaN(first) || isNaN(last) || last <= first) return null;
  return formatDuration(last - first);
}

/**
 * Inline collapsible attached to a session-bridged agent comment, showing
 * just the messages produced during that turn. A turn is the message
 * window (fromExclusive, toInclusive] in a single session — sessions are
 * reused across turns in office tasks, so per-comment slicing is the
 * only way to attribute work to a specific reply.
 */
export function AgentTurnPanel({
  taskId,
  sessionId,
  fromExclusive,
  toInclusive,
  isLive,
}: AgentTurnPanelProps) {
  const { messages, isLoading } = useSessionMessages(sessionId);
  const turnMessages = useMemo(() => {
    const fromMs = fromExclusive ? new Date(fromExclusive).getTime() : -Infinity;
    const toMs = new Date(toInclusive).getTime();
    if (isNaN(toMs)) return [];
    return messages.filter((m) => {
      const ts = new Date(m.created_at).getTime();
      if (isNaN(ts)) return false;
      return ts > fromMs && ts <= toMs;
    });
  }, [messages, fromExclusive, toInclusive]);

  const [open, setOpen] = useState(Boolean(isLive));

  if (turnMessages.length === 0 && !isLoading && !isLive) return null;

  const duration = turnDuration(turnMessages);

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="mt-2">
      <CollapsibleTrigger className="flex items-center gap-2 w-full py-1 text-xs text-muted-foreground cursor-pointer hover:text-foreground transition-colors">
        {isLive ? (
          <IconLoader2 className="h-3 w-3 animate-spin text-primary shrink-0" />
        ) : (
          <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/40 shrink-0" />
        )}
        <span>
          {isLive ? "working" : "worked"}
          {duration && <span className="ml-1">for {duration}</span>}
          {turnMessages.length > 0 && (
            <span className="ml-1">
              · {turnMessages.length} message{turnMessages.length === 1 ? "" : "s"}
            </span>
          )}
        </span>
        <span className="flex-1" />
        <IconChevronDown
          className={`h-3.5 w-3.5 transition-transform ${open ? "rotate-180" : ""}`}
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="ml-4 mt-1 border-l border-border/50 pl-3 flex flex-col gap-2">
          {turnMessages.map((msg, idx) => (
            <MessageRenderer
              key={msg.id}
              comment={msg}
              isTaskDescription={idx === 0 && msg.author_type === "user"}
              taskId={taskId}
              sessionId={sessionId}
            />
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
