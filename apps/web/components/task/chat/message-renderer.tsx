"use client";

import { memo, useState, useCallback, type ReactElement } from "react";
import { IconPlayerPlay } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { Message, TaskSessionState, TaskState } from "@/lib/types/http";
import type { ToolCallMetadata } from "@/components/task/chat/types";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildStartCreatedRequest } from "@/lib/services/session-launch-helpers";
import { useAppStore } from "@/components/state-provider";
import { ChatMessage } from "@/components/task/chat/messages/chat-message";
import { PermissionRequestMessage } from "@/components/task/chat/messages/permission-request-message";
import { StatusMessage } from "@/components/task/chat/messages/status-message";
import { ToolCallMessage } from "@/components/task/chat/messages/tool-call-message";
import { ToolEditMessage } from "@/components/task/chat/messages/tool-edit-message";
import { ToolReadMessage } from "@/components/task/chat/messages/tool-read-message";
import { ToolSearchMessage } from "@/components/task/chat/messages/tool-search-message";
import { ToolExecuteMessage } from "@/components/task/chat/messages/tool-execute-message";
import { ThinkingMessage } from "@/components/task/chat/messages/thinking-message";
import { TodoMessage } from "@/components/task/chat/messages/todo-message";
import { ScriptExecutionMessage } from "@/components/task/chat/messages/script-execution-message";
import { ClarificationRequestMessage } from "@/components/task/chat/messages/clarification-request-message";
import { ToolSubagentMessage } from "@/components/task/chat/messages/tool-subagent-message";
import { MonitorMessage } from "@/components/task/chat/messages/monitor-message";
import { AgentPlanMessage } from "@/components/task/chat/messages/agent-plan-message";
import { ActionMessage } from "@/components/task/chat/messages/action-message";

type AdapterContext = {
  isTaskDescription: boolean;
  sessionState?: TaskSessionState;
  taskState?: TaskState;
  taskId?: string;
  permissionsByToolCallId?: Map<string, Message>;
  childrenByParentToolCallId?: Map<string, Message[]>;
  worktreePath?: string;
  sessionId?: string;
  onOpenFile?: (path: string) => void;
  allMessages?: Message[];
  onScrollToMessage?: (messageId: string) => void;
};

function TaskDescriptionStartButton({ taskId, sessionId }: { taskId: string; sessionId: string }) {
  const [isStarting, setIsStarting] = useState(false);
  const prepareStatus = useAppStore(
    (state) => state.prepareProgress.bySessionId[sessionId]?.status ?? null,
  );

  const handleStart = useCallback(async () => {
    setIsStarting(true);
    try {
      const { request } = buildStartCreatedRequest(taskId, sessionId);
      await launchSession(request);
    } catch (error) {
      console.error("Failed to start agent:", error);
    } finally {
      setIsStarting(false);
    }
  }, [taskId, sessionId]);

  // Hide while environment is being prepared
  if (prepareStatus === "preparing") return null;

  return (
    <div className="flex justify-end mt-1.5">
      <Button
        size="sm"
        variant="default"
        className="cursor-pointer gap-1.5"
        onClick={handleStart}
        disabled={isStarting}
        data-testid="task-description-start-button"
      >
        <IconPlayerPlay className="h-3.5 w-3.5" />
        {isStarting ? "Starting…" : "Start agent"}
      </Button>
    </div>
  );
}

type MessageAdapter = {
  matches: (comment: Message, ctx: AdapterContext) => boolean;
  render: (comment: Message, ctx: AdapterContext) => ReactElement;
};

const adapters: MessageAdapter[] = [
  {
    matches: (comment) => comment.type === "thinking",
    render: (comment) => <ThinkingMessage comment={comment} />,
  },
  {
    matches: (comment) => comment.type === "todo",
    render: (comment) => <TodoMessage comment={comment} />,
  },
  {
    matches: (comment) => comment.type === "tool_edit",
    render: (comment, ctx) => (
      <ToolEditMessage
        comment={comment}
        worktreePath={ctx.worktreePath}
        onOpenFile={ctx.onOpenFile}
      />
    ),
  },
  {
    matches: (comment) => comment.type === "tool_read",
    render: (comment, ctx) => (
      <ToolReadMessage
        comment={comment}
        worktreePath={ctx.worktreePath}
        sessionId={ctx.sessionId}
        onOpenFile={ctx.onOpenFile}
      />
    ),
  },
  {
    matches: (comment) => comment.type === "tool_search",
    render: (comment, ctx) => (
      <ToolSearchMessage
        comment={comment}
        worktreePath={ctx.worktreePath}
        onOpenFile={ctx.onOpenFile}
      />
    ),
  },
  {
    matches: (comment) => comment.type === "tool_execute",
    render: (comment, ctx) => (
      <ToolExecuteMessage comment={comment} worktreePath={ctx.worktreePath} />
    ),
  },
  {
    // Claude-acp's Monitor tool — long-lived background script with streaming
    // events. Rendered with a dedicated card that shows watching state, event
    // count, and the most recent event tail. Detected via the structured
    // `monitor` view the adapter writes into the Generic payload's output
    // wrapper (presence-based rather than title-based so renames upstream
    // don't break the match). Must run BEFORE the generic tool_call adapter.
    matches: (comment) => {
      if (comment.type !== "tool_call") return false;
      const meta = comment.metadata as ToolCallMetadata | undefined;
      const out = meta?.normalized?.generic?.output as { monitor?: unknown } | undefined;
      return !!out && typeof out === "object" && !!out.monitor;
    },
    render: (comment) => <MonitorMessage comment={comment} />,
  },
  {
    // Subagent Task tool calls with nested children
    matches: (comment, ctx) => {
      if (comment.type !== "tool_call") return false;
      const metadata = comment.metadata as ToolCallMetadata | undefined;
      const isSubagent = metadata?.normalized?.kind === "subagent_task";
      const toolCallId = metadata?.tool_call_id;
      const hasChildren = toolCallId
        ? (ctx.childrenByParentToolCallId?.has(toolCallId) ?? false)
        : false;
      return isSubagent || hasChildren;
    },
    render: (comment, ctx) => {
      const toolCallId = (comment.metadata as ToolCallMetadata | undefined)?.tool_call_id;
      const childMessages = toolCallId
        ? (ctx.childrenByParentToolCallId?.get(toolCallId) ?? [])
        : [];

      // Create a render function for child messages
      const renderChild = (child: Message) => {
        // Recursively use MessageRenderer for children (without subagent nesting)
        const childCtx = { ...ctx, childrenByParentToolCallId: undefined };
        const adapter =
          adapters.find((entry) => entry.matches(child, childCtx)) ?? adapters[adapters.length - 1];
        return adapter.render(child, childCtx);
      };

      return (
        <ToolSubagentMessage
          comment={comment}
          childMessages={childMessages}
          worktreePath={ctx.worktreePath}
          onOpenFile={ctx.onOpenFile}
          renderChild={renderChild}
        />
      );
    },
  },
  {
    matches: (comment) => comment.type === "tool_call",
    render: (comment, ctx) => {
      const toolCallId = (comment.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
      const permissionMessage = toolCallId
        ? ctx.permissionsByToolCallId?.get(toolCallId)
        : undefined;
      return (
        <ToolCallMessage
          comment={comment}
          permissionMessage={permissionMessage}
          worktreePath={ctx.worktreePath}
        />
      );
    },
  },
  {
    matches: (comment) => {
      const meta = comment.metadata as Record<string, unknown> | undefined;
      return Array.isArray(meta?.actions) && (meta.actions as unknown[]).length > 0;
    },
    render: (comment, ctx) => <ActionMessage comment={comment} sessionState={ctx.sessionState} />,
  },
  {
    matches: (comment) =>
      comment.type === "error" || comment.type === "status" || comment.type === "progress",
    render: (comment) => <StatusMessage comment={comment} />,
  },
  {
    // Standalone permission requests (no matching tool call)
    matches: (comment) => comment.type === "permission_request",
    render: (comment) => <PermissionRequestMessage comment={comment} />,
  },
  {
    matches: (comment) => comment.type === "clarification_request",
    render: (comment) => <ClarificationRequestMessage comment={comment} />,
  },
  {
    matches: (comment) => comment.type === "agent_plan",
    render: (comment) => <AgentPlanMessage comment={comment} />,
  },
  {
    matches: (comment) => comment.type === "script_execution",
    render: (comment) => <ScriptExecutionMessage comment={comment} />,
  },
  {
    matches: () => true,
    render: (comment, ctx) => {
      if (
        comment.author_type === "user" ||
        (ctx.isTaskDescription && ctx.sessionState !== "FAILED")
      ) {
        const showStartButton =
          ctx.isTaskDescription &&
          ctx.sessionState === "CREATED" &&
          ctx.taskState !== "SCHEDULING" &&
          ctx.taskId &&
          ctx.sessionId;
        return (
          <>
            <ChatMessage
              comment={comment}
              label="You"
              className="bg-primary/10 text-foreground border-primary/30"
              allMessages={ctx.allMessages}
              onScrollToMessage={ctx.onScrollToMessage}
            />
            {showStartButton && (
              <TaskDescriptionStartButton taskId={ctx.taskId!} sessionId={ctx.sessionId!} />
            )}
          </>
        );
      }
      return (
        <ChatMessage
          comment={comment}
          label="Agent"
          className="bg-muted/40 text-foreground border-border/60"
          showRichBlocks={comment.type === "message" || comment.type === "content" || !comment.type}
          allMessages={ctx.allMessages}
          onScrollToMessage={ctx.onScrollToMessage}
        />
      );
    },
  },
];

type MessageRendererProps = {
  comment: Message;
  isTaskDescription: boolean;
  sessionState?: TaskSessionState;
  taskState?: TaskState;
  taskId?: string;
  permissionsByToolCallId?: Map<string, Message>;
  childrenByParentToolCallId?: Map<string, Message[]>;
  worktreePath?: string;
  sessionId?: string;
  onOpenFile?: (path: string) => void;
  allMessages?: Message[];
  onScrollToMessage?: (messageId: string) => void;
};

export const MessageRenderer = memo(function MessageRenderer({
  comment,
  isTaskDescription,
  sessionState,
  taskState,
  taskId,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  worktreePath,
  sessionId,
  onOpenFile,
  allMessages,
  onScrollToMessage,
}: MessageRendererProps) {
  const ctx = {
    isTaskDescription,
    sessionState,
    taskState,
    taskId,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    worktreePath,
    sessionId,
    onOpenFile,
    allMessages,
    onScrollToMessage,
  };
  const adapter =
    adapters.find((entry) => entry.matches(comment, ctx)) ?? adapters[adapters.length - 1];
  return adapter.render(comment, ctx);
});
