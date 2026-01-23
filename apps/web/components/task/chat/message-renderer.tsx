'use client';

import type { ReactElement } from 'react';
import type { Message } from '@/lib/types/http';
import { ChatMessage } from '@/components/task/chat/messages/chat-message';
import { PermissionRequestMessage } from '@/components/task/chat/messages/permission-request-message';
import { StatusMessage } from '@/components/task/chat/messages/status-message';
import { ToolCallMessage } from '@/components/task/chat/messages/tool-call-message';
import { ThinkingMessage } from '@/components/task/chat/messages/thinking-message';
import { TodoMessage } from '@/components/task/chat/messages/todo-message';
import { ScriptExecutionMessage } from '@/components/task/chat/messages/script-execution-message';

type AdapterContext = {
  isTaskDescription: boolean;
  taskId?: string;
  permissionsByToolCallId?: Map<string, Message>;
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
    matches: (comment) => comment.type === 'tool_call',
    render: (comment, ctx) => {
      const toolCallId = (comment.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
      const permissionMessage = toolCallId ? ctx.permissionsByToolCallId?.get(toolCallId) : undefined;
      return <ToolCallMessage comment={comment} permissionMessage={permissionMessage} />;
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
};

export function MessageRenderer({ comment, isTaskDescription, taskId, permissionsByToolCallId }: MessageRendererProps) {
  const ctx = { isTaskDescription, taskId, permissionsByToolCallId };
  const adapter = adapters.find((entry) => entry.matches(comment, ctx)) ?? adapters[adapters.length - 1];
  return adapter.render(comment, ctx);
}
