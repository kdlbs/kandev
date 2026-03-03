"use client";

import { memo, useCallback } from "react";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconX, IconPlus, IconMessageCircle } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { QuickChatContent } from "./quick-chat-content";

type QuickChatModalProps = {
  onNewChat: () => void;
};

function QuickChatTopBar({
  onClose,
  onNewChat,
}: {
  onClose: () => void;
  onNewChat: () => void;
}) {
  return (
    <div className="flex items-center justify-between px-4 py-2 border-b bg-card/50 min-h-[48px]">
      <div className="flex items-center gap-2">
        <IconMessageCircle className="h-4 w-4 text-muted-foreground" />
        <h2 className="text-sm font-medium">Quick Chat</h2>
      </div>
      <div className="flex items-center gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="sm"
              variant="ghost"
              className="px-2 cursor-pointer"
              onClick={onNewChat}
            >
              <IconPlus className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Start new chat</TooltipContent>
        </Tooltip>
        <Button
          size="sm"
          variant="ghost"
          className="px-2 cursor-pointer"
          onClick={onClose}
        >
          <IconX className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

export const QuickChatModal = memo(function QuickChatModal({
  onNewChat,
}: QuickChatModalProps) {
  const isOpen = useAppStore((s) => s.quickChat.isOpen);
  const sessionId = useAppStore((s) => s.quickChat.sessionId);
  const closeQuickChat = useAppStore((s) => s.closeQuickChat);

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) {
        closeQuickChat();
      }
    },
    [closeQuickChat],
  );

  const handleNewChat = useCallback(() => {
    closeQuickChat();
    onNewChat();
  }, [closeQuickChat, onNewChat]);

  if (!sessionId) return null;

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <DialogContent
        className="!max-w-[600px] !w-[600px] max-h-[70vh] h-[70vh] p-0 gap-0 flex flex-col shadow-2xl"
        showCloseButton={false}
        overlayClassName="bg-transparent"
      >
        <DialogTitle className="sr-only">Quick Chat</DialogTitle>
        <QuickChatTopBar onClose={closeQuickChat} onNewChat={handleNewChat} />
        <QuickChatContent sessionId={sessionId} />
      </DialogContent>
    </Dialog>
  );
});

