'use client';

import type { Message } from '@/lib/types/http';
import { ChatMessage } from '@/components/task/chat/messages/chat-message';
import { StatusMessage } from '@/components/task/chat/messages/status-message';
import { ToolCallMessage } from '@/components/task/chat/messages/tool-call-message';
import { ThinkingMessage } from '@/components/task/chat/messages/thinking-message';
import { TodoMessage } from '@/components/task/chat/messages/todo-message';

type AdapterContext = {
  isTaskDescription: boolean;
};

type MessageAdapter = {
  matches: (comment: Message, ctx: AdapterContext) => boolean;
  render: (comment: Message, ctx: AdapterContext) => JSX.Element;
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
    render: (comment) => <ToolCallMessage comment={comment} />,
  },
  {
    matches: (comment) => comment.type === 'error' || comment.type === 'status' || comment.type === 'progress',
    render: (comment) => <StatusMessage comment={comment} />,
  },
  {
    matches: () => true,
    render: (comment, ctx) => {
      if (ctx.isTaskDescription) {
        return (
          <ChatMessage
            comment={comment}
            label="Task"
            className="ml-auto bg-primary/20 text-foreground border border-primary/40"
            showRichBlocks
          />
        );
      }
      if (comment.author_type === 'user') {
        return (
          <ChatMessage
            comment={comment}
            label="You"
            className="ml-auto bg-primary text-primary-foreground"
          />
        );
      }
      return (
        <ChatMessage
          comment={comment}
          label="Agent"
          className="bg-muted text-foreground"
          showRichBlocks={comment.type === 'message' || comment.type === 'content' || !comment.type}
        />
      );
    },
  },
];

export function MessageRenderer({ comment, isTaskDescription }: { comment: Message; isTaskDescription: boolean }) {
  const ctx = { isTaskDescription };
  const adapter = adapters.find((entry) => entry.matches(comment, ctx)) ?? adapters[adapters.length - 1];
  return adapter.render(comment, ctx);
}
