"use client";

import { useState } from "react";
import { IconSend } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { ScrollArea } from "@kandev/ui/scroll-area";

type AdvancedChatPanelProps = {
  taskId: string;
};

export function AdvancedChatPanel({ taskId }: AdvancedChatPanelProps) {
  const [message, setMessage] = useState("");

  return (
    <div className="flex flex-col h-full">
      <ScrollArea className="flex-1 p-4">
        <div className="flex flex-col items-center justify-center h-full min-h-[200px] text-center">
          <p className="text-sm text-muted-foreground">
            No messages yet for task {taskId}.
          </p>
          <p className="text-xs text-muted-foreground mt-1">
            Send a message to start the conversation with the assigned agent.
          </p>
        </div>
      </ScrollArea>
      <div className="border-t border-border p-3 shrink-0">
        <div className="flex gap-2">
          <Input
            placeholder="Send a message..."
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            className="flex-1 text-sm"
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                // TODO: send message via orchestrate API
                setMessage("");
              }
            }}
          />
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="icon"
                className="h-9 w-9 cursor-pointer shrink-0"
                disabled={!message.trim()}
                onClick={() => {
                  // TODO: send message via orchestrate API
                  setMessage("");
                }}
              >
                <IconSend className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Send message</TooltipContent>
          </Tooltip>
        </div>
      </div>
    </div>
  );
}
