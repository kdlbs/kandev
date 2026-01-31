'use client';

import { memo, type ReactElement } from 'react';
import type { Message } from '@/lib/types/http';
import type { ToolCallMetadata } from '@/components/task/chat/types';
import { ChatMessage } from '@/components/task/chat/messages/chat-message';
import { PermissionRequestMessage } from '@/components/task/chat/messages/permission-request-message';
import { StatusMessage } from '@/components/task/chat/messages/status-message';
import { ToolCallMessage } from '@/components/task/chat/messages/tool-call-message';
import { ToolEditMessage } from '@/components/task/chat/messages/tool-edit-message';
import { ToolReadMessage } from '@/components/task/chat/messages/tool-read-message';
import { ToolSearchMessage } from '@/components/task/chat/messages/tool-search-message';
import { ToolExecuteMessage } from '@/components/task/chat/messages/tool-execute-message';
import { ThinkingMessage } from '@/components/task/chat/messages/thinking-message';
import { TodoMessage } from '@/components/task/chat/messages/todo-message';
import { ScriptExecutionMessage } from '@/components/task/chat/messages/script-execution-message';
import { ClarificationRequestMessage } from '@/components/task/chat/messages/clarification-request-message';
import { ToolSubagentMessage } from '@/components/task/chat/messages/tool-subagent-message';

type AdapterContext = {
  isTaskDescription: boolean;
  taskId?: string;
  permissionsByToolCallId?: Map<string, Message>;
  childrenByParentToolCallId?: Map<string, Message[]>;
  worktreePath?: string;
  sessionId?: string;
  onOpenFile?: (path: string) => void;
};

type MessageAdapter = {
  matches: (comment: Message, ctx: AdapterContext) => boolean;
  render: (comment: Message, ctx: AdapterContext) => ReactElement;
};

const adapters: MessageAdapter[] = [
  {
    matches: (comment) => comment.type === 'thinking',
    render: (comment) => <ThinkingMessage comment={comment} />,
  },
  {
    matches: (comment) => comment.type === 'todo',
    render: (comment) => <TodoMessage comment={comment} />,
  },
  {
    matches: (comment) => comment.type === 'tool_edit',
    render: (comment, ctx) => <ToolEditMessage comment={comment} worktreePath={ctx.worktreePath} onOpenFile={ctx.onOpenFile} />,
  },
  {
    matches: (comment) => comment.type === 'tool_read',
    render: (comment, ctx) => <ToolReadMessage comment={comment} worktreePath={ctx.worktreePath} sessionId={ctx.sessionId} onOpenFile={ctx.onOpenFile} />,
  },
  {
    matches: (comment) => comment.type === 'tool_search',
    render: (comment, ctx) => <ToolSearchMessage comment={comment} worktreePath={ctx.worktreePath} onOpenFile={ctx.onOpenFile} />,
  },
  {
    matches: (comment) => comment.type === 'tool_execute',
    render: (comment, ctx) => <ToolExecuteMessage comment={comment} worktreePath={ctx.worktreePath} />,
  },
  {
    // Subagent Task tool calls with nested children
    matches: (comment, ctx) => {
      if (comment.type !== 'tool_call') return false;
      const metadata = comment.metadata as ToolCallMetadata | undefined;
      const isSubagent = metadata?.normalized?.kind === 'subagent_task';
      const toolCallId = metadata?.tool_call_id;
      const hasChildren = toolCallId ? (ctx.childrenByParentToolCallId?.has(toolCallId) ?? false) : false;
      return isSubagent || hasChildren;
    },
    render: (comment, ctx) => {
      const toolCallId = (comment.metadata as ToolCallMetadata | undefined)?.tool_call_id;
      const childMessages = toolCallId ? ctx.childrenByParentToolCallId?.get(toolCallId) ?? [] : [];

      // Create a render function for child messages
      const renderChild = (child: Message) => {
        // Recursively use MessageRenderer for children (without subagent nesting)
        const childCtx = { ...ctx, childrenByParentToolCallId: undefined };
        const adapter = adapters.find((entry) => entry.matches(child, childCtx)) ?? adapters[adapters.length - 1];
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
    matches: (comment) => comment.type === 'tool_call',
    render: (comment, ctx) => {
      const toolCallId = (comment.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
      const permissionMessage = toolCallId ? ctx.permissionsByToolCallId?.get(toolCallId) : undefined;
      return <ToolCallMessage comment={comment} permissionMessage={permissionMessage} worktreePath={ctx.worktreePath} />;
    },
  },
  {
    matches: (comment) => comment.type === 'error' || comment.type === 'status' || comment.type === 'progress',
    render: (comment) => <StatusMessage comment={comment} />,
  },
  {
    // Standalone permission requests (no matching tool call)
    matches: (comment) => comment.type === 'permission_request',
    render: (comment) => <PermissionRequestMessage comment={comment} />,
  },
  {
    matches: (comment) => comment.type === 'clarification_request',
    render: (comment) => <ClarificationRequestMessage comment={comment} />,
  },
  {
    matches: (comment) => comment.type === 'script_execution',
    render: (comment) => <ScriptExecutionMessage comment={comment} />,
  },
  {
    matches: () => true,
    render: (comment, ctx) => {
      if (ctx.isTaskDescription) {
        return (
          <ChatMessage
            comment={comment}
            label="Task"
            className="bg-amber-500/10 text-foreground border-amber-500/30"
            showRichBlocks
          />
        );
      }
      if (comment.author_type === 'user') {
        return (
          <ChatMessage
            comment={comment}
            label="You"
            className="bg-primary/10 text-foreground border-primary/30"
          />
        );
      }
      return (
        <ChatMessage
          comment={comment}
          label="Agent"
          className="bg-muted/40 text-foreground border-border/60"
          showRichBlocks={comment.type === 'message' || comment.type === 'content' || !comment.type}
        />
      );
    },
  },
];

type MessageRendererProps = {
  comment: Message;
  isTaskDescription: boolean;
  taskId?: string;
  permissionsByToolCallId?: Map<string, Message>;
  childrenByParentToolCallId?: Map<string, Message[]>;
  worktreePath?: string;
  sessionId?: string;
  onOpenFile?: (path: string) => void;
};

export const MessageRenderer = memo(function MessageRenderer({
  comment,
  isTaskDescription,
  taskId,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  worktreePath,
  sessionId,
  onOpenFile,
}: MessageRendererProps) {
  const ctx = { isTaskDescription, taskId, permissionsByToolCallId, childrenByParentToolCallId, worktreePath, sessionId, onOpenFile };
  const adapter = adapters.find((entry) => entry.matches(comment, ctx)) ?? adapters[adapters.length - 1];
  return adapter.render(comment, ctx);
});
