"use client";

import { useState, useCallback, memo } from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { cn, transformPathsInText } from "@/lib/utils";
import type { Message, TaskSessionState, TaskState } from "@/lib/types/http";
import type { TurnGroup } from "@/hooks/use-processed-messages";
import type { ToolCallMetadata } from "@/components/task/chat/types";
import { MessageRenderer } from "@/components/task/chat/message-renderer";

type TurnGroupMessageProps = {
  group: TurnGroup;
  sessionId: string | null;
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId?: Map<string, Message[]>;
  taskId?: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  /** Whether this is the last turn group in the current turn */
  isLastGroup?: boolean;
  /** Whether the turn is still active (agent is running) */
  isTurnActive?: boolean;
  allMessages?: Message[];
  sessionState?: TaskSessionState;
  taskState?: TaskState;
  onScrollToMessage?: (messageId: string) => void;
};

function countMessageTypes(messages: Message[]): { toolCalls: number; subagents: number } {
  let toolCalls = 0;
  let subagents = 0;
  for (const msg of messages) {
    const metadata = msg.metadata as ToolCallMetadata | undefined;
    if (metadata?.normalized?.kind === "subagent_task") {
      subagents++;
    } else {
      toolCalls++;
    }
  }
  return { toolCalls, subagents };
}

function getActiveGroupDescription(messages: Message[]): string {
  for (let i = messages.length - 1; i >= 0; i--) {
    const msg = messages[i];
    const metadata = msg.metadata as ToolCallMetadata | undefined;
    if (metadata?.title) return metadata.title;
    if (msg.content && msg.content.length > 0) {
      return msg.content.slice(0, 60) + (msg.content.length > 60 ? "..." : "");
    }
  }
  return "Working...";
}

function getCompletedGroupDescription(messages: Message[]): string {
  const { toolCalls, subagents } = countMessageTypes(messages);
  if (subagents === 0) return `tool call${toolCalls !== 1 ? "s" : ""}`;
  if (toolCalls === 0) return `subagent${subagents !== 1 ? "s" : ""}`;
  return `tool call${toolCalls !== 1 ? "s" : ""}, ${subagents} subagent${subagents !== 1 ? "s" : ""}`;
}

function getGroupDescription(messages: Message[], isActive: boolean): string {
  if (isActive) return getActiveGroupDescription(messages);
  return getCompletedGroupDescription(messages);
}

function hasPendingPermission(
  messages: Message[],
  permissionsByToolCallId: Map<string, Message>,
): boolean {
  for (const msg of messages) {
    if (msg.type !== "tool_call") continue;
    const toolCallId = (msg.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
    if (!toolCallId) continue;
    const permissionMsg = permissionsByToolCallId.get(toolCallId);
    if (!permissionMsg) continue;
    const permStatus = (permissionMsg.metadata as { status?: string } | undefined)?.status;
    if (permStatus !== "approved" && permStatus !== "rejected") return true;
  }
  return false;
}

// Tool message types that have status tracking
const TOOL_MESSAGE_TYPES = new Set([
  "tool_call",
  "tool_edit",
  "tool_read",
  "tool_execute",
  "tool_search",
]);

/**
 * Check if any tool or subagent in the group is still running.
 * A tool/subagent is considered running if it's not in a terminal state (complete or error).
 */
function hasRunningTool(messages: Message[]): boolean {
  for (const msg of messages) {
    const metadata = msg.metadata as ToolCallMetadata | undefined;
    const isToolMessage = msg.type && TOOL_MESSAGE_TYPES.has(msg.type);
    const isSubagent = metadata?.normalized?.kind === "subagent_task";
    if (!isToolMessage && !isSubagent) continue;
    const status = metadata?.status;
    if (status !== "complete" && status !== "error") return true;
  }
  return false;
}

type TurnGroupHeaderProps = {
  isExpanded: boolean;
  count: number;
  description: string;
  isGroupRunning: boolean;
  onToggle: () => void;
};

function TurnGroupHeader({
  isExpanded,
  count,
  description,
  isGroupRunning,
  onToggle,
}: TurnGroupHeaderProps) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className={cn(
        "flex items-center gap-2 w-full text-left px-2 py-1.5 -mx-2 rounded",
        "hover:bg-muted/30 transition-colors cursor-pointer",
      )}
    >
      {isExpanded ? (
        <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground/60 flex-shrink-0" />
      ) : (
        <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground/60 flex-shrink-0" />
      )}
      <span className="bg-muted text-muted-foreground text-xs px-1.5 rounded min-w-[20px] text-center font-mono">
        {count}
      </span>
      <span className="font-mono text-xs truncate text-muted-foreground inline-flex items-center gap-1.5">
        {description}
        {isGroupRunning && <GridSpinner className="text-muted-foreground shrink-0" />}
      </span>
    </button>
  );
}

type TurnGroupContentProps = {
  group: TurnGroup;
  sessionId: string | null;
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId?: Map<string, Message[]>;
  taskId?: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  allMessages?: Message[];
  sessionState?: TaskSessionState;
  taskState?: TaskState;
  onScrollToMessage?: (messageId: string) => void;
};

const MIN_REPEAT_TOOL_RUN = 4;

type TurnGroupContentEntry =
  | { kind: "message"; message: Message }
  | { kind: "repeated_tool_summary"; id: string; messages: Message[] };

type ShellExecSummary = {
  command: string;
  workDir: string;
  exitCode: number;
  stdout: string;
  stderr: string;
};

type ShellExecPayload = NonNullable<NonNullable<ToolCallMetadata["normalized"]>["shell_exec"]>;

function getCompleteShellExec(message: Message): ShellExecPayload | null {
  if (message.type !== "tool_execute") return null;
  const metadata = message.metadata as ToolCallMetadata | undefined;
  if (metadata?.status !== "complete") return null;
  return metadata.normalized?.shell_exec ?? null;
}

function readZeroExitCode(shellExec: ShellExecPayload): number | null {
  const output = shellExec?.output;
  const exitCode = output?.exit_code ?? 0;
  return exitCode === 0 ? exitCode : null;
}

function readShellExecSummary(message: Message): ShellExecSummary | null {
  const shellExec = getCompleteShellExec(message);
  if (!shellExec) return null;
  const exitCode = readZeroExitCode(shellExec);
  if (exitCode === null) return null;
  const output = shellExec.output;
  return {
    command: shellExec?.command ?? message.content,
    workDir: shellExec?.work_dir ?? "",
    exitCode,
    stdout: output?.stdout ?? "",
    stderr: output?.stderr ?? "",
  };
}

function repeatFingerprint(message: Message): string | null {
  const summary = readShellExecSummary(message);
  return summary ? JSON.stringify(summary) : null;
}

function compactRepeatRun(entries: TurnGroupContentEntry[], run: Message[]) {
  if (run.length >= MIN_REPEAT_TOOL_RUN) {
    entries.push({ kind: "message", message: run[0] });
    entries.push({
      kind: "repeated_tool_summary",
      id: `repeated-tools-${run[1].id}`,
      messages: run.slice(1, -1),
    });
    entries.push({ kind: "message", message: run[run.length - 1] });
    return;
  }
  for (const message of run) entries.push({ kind: "message", message });
}

export function compactTurnGroupMessages(messages: Message[]): TurnGroupContentEntry[] {
  const entries: TurnGroupContentEntry[] = [];
  let repeatRun: Message[] = [];
  let currentFingerprint: string | null = null;

  const flushRun = () => {
    if (repeatRun.length === 0) return;
    compactRepeatRun(entries, repeatRun);
    repeatRun = [];
    currentFingerprint = null;
  };

  for (const message of messages) {
    const fingerprint = repeatFingerprint(message);
    if (!fingerprint) {
      flushRun();
      entries.push({ kind: "message", message });
      continue;
    }
    if (currentFingerprint === fingerprint) {
      repeatRun.push(message);
      continue;
    }
    flushRun();
    currentFingerprint = fingerprint;
    repeatRun = [message];
  }
  flushRun();
  return entries;
}

type MessageRenderProps = Omit<TurnGroupContentProps, "group">;

function renderMessageEntry(message: Message, props: MessageRenderProps) {
  return (
    <MessageRenderer
      key={message.id}
      comment={message}
      isTaskDescription={false}
      taskId={props.taskId}
      sessionState={props.sessionState}
      taskState={props.taskState}
      permissionsByToolCallId={props.permissionsByToolCallId}
      childrenByParentToolCallId={props.childrenByParentToolCallId}
      worktreePath={props.worktreePath}
      sessionId={props.sessionId ?? undefined}
      onOpenFile={props.onOpenFile}
      allMessages={props.allMessages}
      onScrollToMessage={props.onScrollToMessage}
    />
  );
}

function RepeatedToolSummary({
  entry,
  renderProps,
}: {
  entry: Extract<TurnGroupContentEntry, { kind: "repeated_tool_summary" }>;
  renderProps: MessageRenderProps;
}) {
  const [expanded, setExpanded] = useState(false);
  const count = entry.messages.length;
  return (
    <div data-testid="repeated-tool-summary" className="text-xs text-muted-foreground">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded((prev) => !prev)}
        className={cn(
          "flex w-full items-center gap-2 rounded px-2 py-1 -mx-2 text-left",
          "hover:bg-muted/40 transition-colors",
        )}
      >
        {expanded ? (
          <IconChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60" />
        ) : (
          <IconChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60" />
        )}
        <span className="min-w-0 break-words">
          {count} repeated identical terminal commands {expanded ? "shown" : "hidden"}
        </span>
      </button>
      {expanded && (
        <div className="mt-1 space-y-2">
          {entry.messages.map((msg) => renderMessageEntry(msg, renderProps))}
        </div>
      )}
    </div>
  );
}

function TurnGroupContent({
  group,
  sessionId,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  worktreePath,
  onOpenFile,
  allMessages,
  sessionState,
  taskState,
  onScrollToMessage,
}: TurnGroupContentProps) {
  const renderProps: MessageRenderProps = {
    sessionId,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    taskId,
    worktreePath,
    onOpenFile,
    allMessages,
    sessionState,
    taskState,
    onScrollToMessage,
  };
  return (
    <div className="ml-2 pl-4 border-l-2 border-border/30 mt-1 space-y-2">
      {compactTurnGroupMessages(group.messages).map((entry) =>
        entry.kind === "message" ? (
          renderMessageEntry(entry.message, renderProps)
        ) : (
          <RepeatedToolSummary key={entry.id} entry={entry} renderProps={renderProps} />
        ),
      )}
    </div>
  );
}

export const TurnGroupMessage = memo(function TurnGroupMessage({
  group,
  sessionId,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  worktreePath,
  onOpenFile,
  isLastGroup = false,
  isTurnActive = false,
  allMessages,
  sessionState,
  taskState,
  onScrollToMessage,
}: TurnGroupMessageProps) {
  const isGroupRunning = hasRunningTool(group.messages);
  const hasPending = hasPendingPermission(group.messages, permissionsByToolCallId);

  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);

  // Auto behavior: expand if running, has pending, or is the last group while turn is active
  const autoExpanded = isGroupRunning || hasPending || (isTurnActive && isLastGroup);
  const isExpanded = manualExpandState ?? autoExpanded;

  const handleToggle = useCallback(() => {
    setManualExpandState((prev) => !(prev ?? autoExpanded));
  }, [autoExpanded]);

  const rawDescription = getGroupDescription(group.messages, isGroupRunning);
  const description = transformPathsInText(rawDescription, worktreePath);
  const count = group.messages.length;

  return (
    <div className="w-full">
      <TurnGroupHeader
        isExpanded={isExpanded}
        count={count}
        description={description}
        isGroupRunning={isGroupRunning}
        onToggle={handleToggle}
      />
      {isExpanded && (
        <TurnGroupContent
          group={group}
          sessionId={sessionId}
          permissionsByToolCallId={permissionsByToolCallId}
          childrenByParentToolCallId={childrenByParentToolCallId}
          taskId={taskId}
          worktreePath={worktreePath}
          onOpenFile={onOpenFile}
          allMessages={allMessages}
          sessionState={sessionState}
          taskState={taskState}
          onScrollToMessage={onScrollToMessage}
        />
      )}
    </div>
  );
});
