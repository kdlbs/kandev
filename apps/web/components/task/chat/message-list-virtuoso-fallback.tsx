"use client";

import { SessionPanelContent } from "@kandev/ui/pannel-session";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import { AgentStatus } from "@/components/task/chat/messages/agent-status";
import type { Message, TaskSessionState } from "@/lib/types/http";
import { MessageListStatus } from "./message-list-shared";

export function VirtuosoMessageListFallback(props: {
  isLoadingMore: boolean;
  hasMore: boolean;
  showLoadingState: boolean;
  messagesLoading: boolean;
  isInitialLoading: boolean;
  messages: Message[];
  loadMore: () => Promise<number>;
  sessionState?: TaskSessionState;
  sessionId: string | null;
  footerActions: Message[];
}) {
  return (
    <SessionPanelContent className="relative p-4 chat-message-list">
      <MessageListStatus
        isLoadingMore={props.isLoadingMore}
        hasMore={props.hasMore}
        showLoadingState={props.showLoadingState}
        messagesLoading={props.messagesLoading}
        isInitialLoading={props.isInitialLoading}
        messagesCount={props.messages.length}
        onLoadMore={props.loadMore}
      />
      <AgentStatus
        sessionState={props.sessionState}
        sessionId={props.sessionId}
        messages={props.messages}
      />
      {props.footerActions.map((message) => (
        <MessageRenderer key={message.id} comment={message} isTaskDescription={false} />
      ))}
    </SessionPanelContent>
  );
}
