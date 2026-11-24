"use client";

import { forwardRef, type MouseEvent, type RefObject } from "react";
import { captureQuickChatLauncherFocus } from "@/components/quick-chat/quick-chat-focus";
import TaskLink, { type TaskLinkProps } from "@/components/routing/task-link";
import { useOptionalAppStoreApi } from "@/components/state-provider";

type InboxOpenTaskLinkProps = TaskLinkProps & {
  returnFocusRef?: RefObject<HTMLElement | null>;
};

function isSameTabActivation(event: MouseEvent<HTMLAnchorElement>): boolean {
  const { target } = event.currentTarget;
  return (
    !event.defaultPrevented &&
    event.button === 0 &&
    !event.metaKey &&
    !event.ctrlKey &&
    !event.shiftKey &&
    !event.altKey &&
    (!target || target === "_self")
  );
}

export const InboxOpenTaskLink = forwardRef<HTMLAnchorElement, InboxOpenTaskLinkProps>(
  function InboxOpenTaskLink({ taskId, sessionId, returnFocusRef, onClick, ...props }, ref) {
    const store = useOptionalAppStoreApi();

    const handleClick = (event: MouseEvent<HTMLAnchorElement>) => {
      onClick?.(event);
      if (!store || !isSameTabActivation(event)) return;

      const { workspaces, quickChat, openQuickChat } = store.getState();
      const conversation = quickChat.sessions.find(
        (entry) =>
          entry.kind === "chat" &&
          entry.workspaceId === workspaces.activeId &&
          entry.taskId === taskId &&
          (!sessionId || entry.sessionId === sessionId),
      );
      if (!conversation) return;

      event.preventDefault();
      captureQuickChatLauncherFocus({
        returnFocusRef: returnFocusRef ?? { current: event.currentTarget },
      });
      openQuickChat(
        conversation.sessionId,
        conversation.workspaceId,
        conversation.agentProfileId,
        conversation.kind,
        conversation.taskId,
      );
    };

    return (
      <TaskLink
        {...props}
        ref={ref}
        taskId={taskId}
        sessionId={sessionId ?? undefined}
        onClick={handleClick}
      />
    );
  },
);
