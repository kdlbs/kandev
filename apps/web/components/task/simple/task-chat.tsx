"use client";

import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { toast } from "sonner";
import { IconCode, IconChevronDown, IconSend, IconUpload, IconUser } from "@tabler/icons-react";
import { AgentAvatar } from "@/app/office/components/agent-avatar";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { useAppStore } from "@/components/state-provider";
import { selectCommandCount } from "@/lib/state/slices/session/selectors";
import { createComment } from "@/lib/api/domains/office-api";
import { formatRelativeTime } from "@/lib/utils";
import { MarkdownComment } from "./markdown-comment";
import { AgentTurnPanel } from "./components/agent-turn-panel";
import { RunErrorEntry } from "./components/run-error-entry";
import { UserCommentRunBadge } from "./components/user-comment-run-badge";
import { buildCommentTurnContext, type CommentTurnContext } from "./turn-context";
import { groupSessionsForTimeline, groupSortKey, type SessionGroup } from "./session-groups";
import type {
  TaskComment,
  TaskDecision,
  TaskSession,
  TimelineEvent,
} from "@/app/office/tasks/[id]/types";
import {
  buildLaterAgentReplyMap,
  buildRunErrorsFromSessions,
  mergeChatEntries,
  type ChatEntry,
} from "./chat-entries";

const MAX_INLINE_SESSIONS = 50;
const AUTOSCROLL_THRESHOLD_PX = 80;

type TaskChatProps = {
  taskId: string;
  comments: TaskComment[];
  timeline?: TimelineEvent[];
  sessions?: TaskSession[];
  decisions?: TaskDecision[];
  reviewers?: string[];
  approvers?: string[];
  scrollParent?: HTMLElement | null;
  readOnly?: boolean;
  onCommentsChanged?: () => void;
  taskTitle?: string;
  taskDescription?: string;
};

function partitionGroups(groups: SessionGroup[]): {
  visible: SessionGroup[];
  older: SessionGroup[];
} {
  if (groups.length <= MAX_INLINE_SESSIONS) {
    return { visible: groups, older: [] };
  }
  // Sort ascending by representative start, keep the most recent N.
  const sorted = [...groups].sort((a, b) => groupSortKey(a).localeCompare(groupSortKey(b)));
  const cutoff = sorted.length - MAX_INLINE_SESSIONS;
  return { older: sorted.slice(0, cutoff), visible: sorted.slice(cutoff) };
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const seconds = Math.round(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remaining = seconds % 60;
  return `${minutes}m ${remaining}s`;
}

function CommentEntry({
  comment,
  taskId,
  turn,
  hasLaterAgentReply,
}: {
  comment: TaskComment;
  taskId: string;
  turn?: CommentTurnContext;
  hasLaterAgentReply?: boolean;
}) {
  const isAgent = comment.authorType === "agent";
  // Resolve the agent name from the office agents store so renames
  // flow through automatically. Backend session-bridged comments don't
  // carry a name; the mapper leaves authorName empty for agents.
  const resolvedAgentName = useAppStore((s) =>
    isAgent
      ? (s.office.agentProfiles.find((a) => a.id === comment.authorId)?.name ??
        comment.authorName ??
        "Agent")
      : "",
  );
  const displayName = isAgent ? resolvedAgentName : "You";
  return (
    <div
      id={`comment-${comment.id}`}
      className="flex gap-3 py-3 border-b border-border/50 scroll-mt-16"
    >
      {isAgent ? (
        <AgentAvatar name={displayName} size="md" />
      ) : (
        <div className="h-8 w-8 rounded-md bg-muted flex items-center justify-center shrink-0">
          <IconUser className="h-4 w-4 text-muted-foreground" />
        </div>
      )}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="font-medium text-sm">{displayName}</span>
          {isAgent && comment.source === "session" && (
            <span className="text-xs text-muted-foreground">via session</span>
          )}
          {comment.status && (
            <span className="text-xs text-muted-foreground">
              {comment.status}
              {comment.durationMs != null && ` after ${formatDuration(comment.durationMs)}`}
            </span>
          )}
          <span className="text-xs text-muted-foreground">
            {formatRelativeTime(comment.createdAt)}
          </span>
        </div>
        <div className="mt-1">
          <MarkdownComment content={comment.content} />
        </div>
        {!isAgent &&
          comment.runStatus &&
          comment.runStatus !== "finished" &&
          !hasLaterAgentReply && (
            <div className="mt-1.5">
              <UserCommentRunBadge status={comment.runStatus} errorMessage={comment.runError} />
            </div>
          )}
        {comment.toolCalls && comment.toolCalls.length > 0 && (
          <Collapsible>
            <CollapsibleTrigger className="flex items-center gap-1 text-xs text-muted-foreground mt-1 cursor-pointer hover:text-foreground transition-colors">
              <IconCode className="h-3 w-3" />
              Worked -- ran {comment.toolCalls.length} commands
              <IconChevronDown className="h-3 w-3" />
            </CollapsibleTrigger>
            <CollapsibleContent>
              <div className="mt-2 space-y-1 text-xs font-mono bg-muted rounded-md p-2">
                {comment.toolCalls.map((tc) => (
                  <div key={tc.id} className="text-muted-foreground">
                    <span className="text-foreground">{tc.name}</span>
                    {tc.input && <span className="ml-1 opacity-70">{tc.input}</span>}
                  </div>
                ))}
              </div>
            </CollapsibleContent>
          </Collapsible>
        )}
        {turn && (
          <AgentTurnPanel
            taskId={taskId}
            sessionId={turn.sessionId}
            fromExclusive={turn.fromExclusive}
            toInclusive={comment.createdAt}
          />
        )}
      </div>
    </div>
  );
}

// formatDecisionLine renders the human-readable summary for a single
// task decision entry in the timeline. Approve rows read like
// "CEO approved this task"; request-changes rows append the comment.
export function formatDecisionLine(decision: TaskDecision): string {
  const who = decision.deciderName?.trim() || "Someone";
  if (decision.decision === "approved") {
    return `${who} approved this task`;
  }
  const tail = decision.comment ? `: '${decision.comment}'` : "";
  return `${who} requested changes${tail}`;
}

function DecisionTimelineEntry({ decision }: { decision: TaskDecision }) {
  const isApproval = decision.decision === "approved";
  return (
    <div
      className="flex items-center gap-2 px-4 py-1.5 text-xs text-muted-foreground"
      data-testid="decision-timeline-entry"
    >
      <span className={isApproval ? "text-green-600" : "text-red-600"}>
        {isApproval ? "approved" : "requested changes"}
      </span>
      <span className="truncate">{formatDecisionLine(decision)}</span>
      <span className="ml-auto shrink-0">{formatRelativeTime(decision.createdAt)}</span>
    </div>
  );
}

function TimelineEntry({ event }: { event: TimelineEvent }) {
  return (
    <div className="flex items-center gap-2 px-4 py-1.5 text-xs text-muted-foreground">
      {event.type === "status_change" && event.from && event.to ? (
        <span>
          Status changed from <strong>{event.from}</strong> to <strong>{event.to}</strong>
        </span>
      ) : (
        <span>{event.type.replaceAll("_", " ")}</span>
      )}
      <span className="ml-auto shrink-0">{formatRelativeTime(event.at)}</span>
    </div>
  );
}

async function readFileAsMarkdown(file: File): Promise<string> {
  if (file.type.startsWith("image/")) {
    const dataUrl = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result as string);
      reader.onerror = reject;
      reader.readAsDataURL(file);
    });
    return `![${file.name}](${dataUrl})`;
  }
  const text = await file.text();
  return `**${file.name}**\n\`\`\`\n${text.slice(0, 4000)}\n\`\`\``;
}

function useChatInputHandlers(setInput: React.Dispatch<React.SetStateAction<string>>) {
  const processFiles = useCallback(
    async (files: File[]) => {
      for (const file of files) {
        try {
          const md = await readFileAsMarkdown(file);
          setInput((prev) => (prev ? `${prev}\n\n${md}` : md));
        } catch {
          toast.error(`Failed to read ${file.name}`);
        }
      }
    },
    [setInput],
  );

  const handleFileSelect = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files;
      if (!files || files.length === 0) return;
      await processFiles(Array.from(files));
      e.target.value = "";
    },
    [processFiles],
  );

  const handlePaste = useCallback(
    async (e: React.ClipboardEvent) => {
      const items = e.clipboardData?.items;
      if (!items) return;
      const imageFiles: File[] = [];
      for (const item of Array.from(items)) {
        if (item.type.startsWith("image/")) {
          const file = item.getAsFile();
          if (file) imageFiles.push(file);
        }
      }
      if (imageFiles.length > 0) {
        e.preventDefault();
        await processFiles(imageFiles);
      }
    },
    [processFiles],
  );

  return { handleFileSelect, handlePaste };
}

type ChatInputProps = {
  taskId: string;
  taskTitle?: string;
  taskDescription?: string;
  onSubmitted?: () => void;
};

function ChatInput({ taskId, taskTitle, taskDescription, onSubmitted }: ChatInputProps) {
  const [input, setInput] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const isUtilityConfigured = useIsUtilityConfigured();
  const { enhancePrompt, isEnhancingPrompt } = useUtilityAgentGenerator({
    sessionId: null,
    taskTitle: taskTitle ?? "",
    taskDescription: taskDescription ?? "",
  });
  const { handleFileSelect, handlePaste } = useChatInputHandlers(setInput);

  const handleSubmit = useCallback(async () => {
    if (!input.trim() || submitting) return;
    setSubmitting(true);
    try {
      await createComment(taskId, { body: input.trim(), author_type: "user" });
      setInput("");
      onSubmitted?.();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to send comment");
    } finally {
      setSubmitting(false);
    }
  }, [input, submitting, taskId, onSubmitted]);

  const handleEnhance = useCallback(() => {
    if (!input.trim()) return;
    enhancePrompt(input, (enhanced: string) => setInput(enhanced));
  }, [input, enhancePrompt]);

  return (
    <div className="mt-4 pt-4 border-t border-border">
      <div className="rounded-md border bg-muted/30 focus-within:ring-1 focus-within:ring-ring">
        <textarea
          ref={textareaRef}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              void handleSubmit();
            }
          }}
          onPaste={handlePaste}
          placeholder="Add a comment..."
          rows={2}
          className="w-full bg-transparent px-3 py-2 text-sm outline-none resize-none"
        />
        <div className="flex items-center gap-1 px-2 pb-2">
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={handleFileSelect}
          />
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                size="icon"
                variant="ghost"
                className="h-7 w-7 cursor-pointer"
                onClick={() => fileInputRef.current?.click()}
              >
                <IconUpload className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Attach file</TooltipContent>
          </Tooltip>
          <EnhancePromptButton
            onClick={handleEnhance}
            isLoading={isEnhancingPrompt}
            isConfigured={isUtilityConfigured}
          />
          <span className="flex-1" />
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                size="icon"
                className="h-7 w-7 cursor-pointer"
                disabled={submitting || !input.trim()}
                onClick={() => void handleSubmit()}
              >
                <IconSend className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Send comment</TooltipContent>
          </Tooltip>
        </div>
      </div>
    </div>
  );
}

function isAtBottom(scrollParent: HTMLElement | null): boolean {
  if (!scrollParent) return true; // window scroll case — be conservative.
  const remaining = scrollParent.scrollHeight - scrollParent.scrollTop - scrollParent.clientHeight;
  return remaining <= AUTOSCROLL_THRESHOLD_PX;
}

function scrollToBottom(scrollParent: HTMLElement | null): void {
  if (!scrollParent) return;
  scrollParent.scrollTop = scrollParent.scrollHeight;
}

/**
 * Auto-scroll the chat container to the bottom when new content arrives,
 * but only if the user was already near the bottom (within ~80px) at the
 * time of the change.
 *
 * Triggers on:
 *   - a new active session entry first appearing (active count grows)
 *   - new messages arriving in any session for this task
 *
 * Uses a scroll listener to track the user's "at-bottom" intent. Reads
 * the latest value before scrolling so we never yank focus from a user
 * who has scrolled up.
 */
function useChatAutoScroll(
  scrollParent: HTMLElement | null,
  sessions: TaskSession[],
  taskId: string,
): void {
  const activeSessionCount = sessions.filter(
    (s) => s.state === "RUNNING" || s.state === "WAITING_FOR_INPUT",
  ).length;

  // Sum messages + command counts across all task sessions — single scalar
  // that grows whenever new content streams in.
  const totalContentSignal = useAppStore((s) => {
    let sum = 0;
    for (const session of sessions) {
      sum += s.messages.bySession[session.id]?.length ?? 0;
      sum += selectCommandCount(s, session.id);
    }
    return sum;
  });

  const wasAtBottomRef = useRef(true);

  useEffect(() => {
    if (!scrollParent) return;
    const handler = () => {
      wasAtBottomRef.current = isAtBottom(scrollParent);
    };
    handler();
    scrollParent.addEventListener("scroll", handler, { passive: true });
    return () => scrollParent.removeEventListener("scroll", handler);
  }, [scrollParent]);

  useEffect(() => {
    if (wasAtBottomRef.current) {
      scrollToBottom(scrollParent);
      // After programmatic scroll, we are still "at bottom" by definition.
      wasAtBottomRef.current = true;
    }
  }, [scrollParent, activeSessionCount, totalContentSignal, taskId]);
}

/**
 * Scrolls the comment matching `location.hash` (e.g. `#comment-cm-A`)
 * into view once it has rendered. Runs whenever the comments list
 * changes so deeplinks land on the target even when comments load
 * after first paint. Cleared after first match so it doesn't fight
 * the user when they scroll away.
 */
function useCommentHashScroll(comments: TaskComment[]): void {
  const targetIdRef = useRef<string | null>(null);
  useEffect(() => {
    if (typeof window === "undefined") return;
    const hash = window.location.hash;
    if (!hash.startsWith("#comment-")) return;
    targetIdRef.current = hash.slice(1);
  }, []);

  useEffect(() => {
    const targetId = targetIdRef.current;
    if (!targetId) return;
    const el = document.getElementById(targetId);
    if (!el) return;
    el.scrollIntoView({ behavior: "smooth", block: "center" });
    targetIdRef.current = null;
  }, [comments]);
}

function ChatEntries({ taskId, entries }: { taskId: string; entries: ChatEntry[] }) {
  return (
    <>
      {entries.map((entry) => {
        if (entry.kind === "comment") {
          return (
            <CommentEntry
              key={`c-${entry.data.id}`}
              comment={entry.data}
              taskId={taskId}
              turn={entry.turn}
              hasLaterAgentReply={entry.hasLaterAgentReply}
            />
          );
        }
        if (entry.kind === "timeline") {
          return <TimelineEntry key={`t-${entry.data.at}-${entry.data.type}`} event={entry.data} />;
        }
        if (entry.kind === "decision") {
          return <DecisionTimelineEntry key={`d-${entry.data.id}`} decision={entry.data} />;
        }
        if (entry.kind === "error") {
          return <RunErrorEntry key={`re-${entry.data.id}`} taskId={taskId} error={entry.data} />;
        }
        // Session entries are intentionally not rendered in the Chat
        // tab: every agent message below already shows the same agent
        // name + "worked for Xs" footer, so the collapsed session
        // header is pure duplicate noise. The per-agent tabs render the
        // full transcript when the user wants it.
        return null;
      })}
    </>
  );
}

export function TaskChat({
  taskId,
  comments,
  timeline = [],
  sessions = [],
  decisions = [],
  reviewers = [],
  approvers = [],
  scrollParent,
  readOnly = false,
  onCommentsChanged,
  taskTitle,
  taskDescription,
}: TaskChatProps) {
  const [showOlder, setShowOlder] = useState(false);
  const groups = useMemo(
    () => groupSessionsForTimeline(sessions, reviewers, approvers),
    [sessions, reviewers, approvers],
  );
  // Per office-task-session-lifecycle/spec.md §G, office (per-agent) groups
  // render inline in the chat timeline alongside any kanban / quick-chat
  // groups — one collapsible entry per (task, agent) pair, ordered by
  // most-recent activity. The per-agent sibling tabs duplicate the same
  // chat for users who want a focused per-agent view.
  const { visible: visibleGroups, older: olderGroups } = useMemo(
    () => partitionGroups(groups),
    [groups],
  );

  const renderedGroups = useMemo(
    () => (showOlder ? [...olderGroups, ...visibleGroups] : visibleGroups),
    [showOlder, olderGroups, visibleGroups],
  );
  const turnCtx = useMemo(() => buildCommentTurnContext(comments, sessions), [comments, sessions]);
  const runErrors = useMemo(() => buildRunErrorsFromSessions(sessions), [sessions]);
  const laterAgentReplyMap = useMemo(() => buildLaterAgentReplyMap(comments), [comments]);
  const entries = useMemo(
    () =>
      mergeChatEntries({
        comments,
        timeline,
        groups: renderedGroups,
        decisions,
        turnCtx,
        runErrors,
        laterAgentReplyMap,
      }),
    [comments, timeline, renderedGroups, decisions, turnCtx, runErrors, laterAgentReplyMap],
  );

  useChatAutoScroll(scrollParent ?? null, sessions, taskId);
  useCommentHashScroll(comments);

  const showOlderToggle = olderGroups.length > 0 && !showOlder;
  const isEmpty = entries.length === 0;

  return (
    <div className="flex flex-col" data-testid="task-chat-root">
      {showOlderToggle && (
        <button
          type="button"
          onClick={() => setShowOlder(true)}
          className="self-start text-xs text-muted-foreground hover:text-foreground underline cursor-pointer mb-2"
          data-testid="show-older-sessions"
        >
          Show {olderGroups.length} older session{olderGroups.length === 1 ? "" : "s"}
        </button>
      )}
      {isEmpty ? (
        <p className="text-sm text-muted-foreground py-4">No comments yet</p>
      ) : (
        <div data-testid="task-chat-entries">
          <ChatEntries taskId={taskId} entries={entries} />
        </div>
      )}
      {!readOnly && (
        <ChatInput
          taskId={taskId}
          taskTitle={taskTitle}
          taskDescription={taskDescription}
          onSubmitted={onCommentsChanged}
        />
      )}
    </div>
  );
}
