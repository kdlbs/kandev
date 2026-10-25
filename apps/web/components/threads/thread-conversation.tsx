"use client";

import { TaskChatPanel } from "@/components/task/task-chat-panel";

/**
 * One column's live conversation.
 *
 * Deliberately passes no `onSend`. `TaskChatPanel`'s own submit path
 * (`useMessageHandler`) is what honours the session's queue input mode, the
 * selected model, plan mode, context files and the optimistic store update; a
 * custom sender here would post straight to `message.add` and silently drop all
 * of it, so a reply to a busy thread would jump its queue.
 *
 * `isVisible` stays false for the same reason the kanban preview keeps it
 * false: a wall of columns is a glance across running work, and letting every
 * mounted column advance its own Slack-style read cursor would mark threads
 * read that the user never looked at.
 */
export function ThreadConversation({ taskId, sessionId }: { taskId: string; sessionId: string }) {
  return (
    <TaskChatPanel
      sessionId={sessionId}
      taskId={taskId}
      hideSessionsDropdown
      embedded
      isVisible={false}
    />
  );
}
