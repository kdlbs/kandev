"use client";

import { useState, useCallback, useRef, useEffect } from "react";
import { IconSend, IconPlayerPlay, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Input } from "@kandev/ui/input";
import { useSessionMessages } from "@/hooks/domains/session/use-session-messages";
import { useSession } from "@/hooks/domains/session/use-session";
import { useSessionLaunch } from "@/hooks/domains/session/use-session-launch";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import type { Message } from "@/lib/types/http";

type AdvancedChatPanelProps = {
  taskId: string;
  sessionId: string | null;
};

export function AdvancedChatPanel({ taskId, sessionId }: AdvancedChatPanelProps) {
  const [message, setMessage] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);

  const { session } = useSession(sessionId);
  const { messages, isLoading } = useSessionMessages(sessionId);
  const agentProfiles = useAppStore((s) => s.agentProfiles.items ?? []);
  const defaultProfile = agentProfiles[0] ?? null;

  const { launch, isLoading: isLaunching } = useSessionLaunch();

  const sessionState = session?.state ?? null;
  const isAgentBusy = sessionState === "RUNNING" || sessionState === "STARTING";
  const canSend = sessionId !== null && (sessionState === "WAITING_FOR_INPUT" || isAgentBusy);

  // Auto-scroll when new messages arrive
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages.length]);

  const handleSend = useCallback(async () => {
    const text = message.trim();
    if (!text) return;

    // No session yet: launch a new one with the first message as prompt
    if (!sessionId) {
      if (!defaultProfile) return;
      const { request } = buildStartRequest(taskId, defaultProfile.id, {
        prompt: text,
        autoStart: true,
      });
      setMessage("");
      await launch(request);
      return;
    }

    // Session exists: send via WS message.add
    const client = getWebSocketClient();
    if (!client) return;
    setMessage("");
    await client.request(
      "message.add",
      { task_id: taskId, session_id: sessionId, content: text },
      10_000,
    );
  }, [message, sessionId, taskId, defaultProfile, launch]);

  const handleStartSession = useCallback(async () => {
    if (!defaultProfile) return;
    const { request } = buildStartRequest(taskId, defaultProfile.id, {
      prompt: "",
      autoStart: true,
    });
    await launch(request);
  }, [taskId, defaultProfile, launch]);

  // No session and no messages: show start prompt
  if (!sessionId && messages.length === 0) {
    return (
      <div className="flex flex-col h-full">
        <div className="flex-1 flex flex-col items-center justify-center p-6 text-center">
          <p className="text-sm text-muted-foreground mb-1">No active session for this task.</p>
          <p className="text-xs text-muted-foreground mb-4">
            Start a session or send a message to begin.
          </p>
          {defaultProfile && (
            <Button
              size="sm"
              className="cursor-pointer gap-1.5"
              onClick={handleStartSession}
              disabled={isLaunching}
            >
              {isLaunching ? (
                <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <IconPlayerPlay className="h-3.5 w-3.5" />
              )}
              {isLaunching ? "Starting..." : "Start session"}
            </Button>
          )}
        </div>
        <ChatInput
          message={message}
          setMessage={setMessage}
          onSend={handleSend}
          disabled={!defaultProfile}
          placeholder={
            defaultProfile ? "Send a message to start a session..." : "No agent profile configured"
          }
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <div ref={scrollRef} className="flex-1 overflow-y-auto p-4">
        {isLoading && messages.length === 0 ? (
          <div className="flex items-center justify-center py-8">
            <IconLoader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {messages.map((msg: Message, idx: number) => (
              <MessageRenderer
                key={msg.id}
                comment={msg}
                isTaskDescription={idx === 0 && msg.author_type === "user"}
                sessionState={sessionState ?? undefined}
                taskId={taskId}
                sessionId={sessionId ?? undefined}
              />
            ))}
          </div>
        )}
      </div>
      <ChatInput
        message={message}
        setMessage={setMessage}
        onSend={handleSend}
        disabled={!canSend && sessionId !== null}
        placeholder={
          isAgentBusy ? "Agent is working... message will be queued" : "Send a message..."
        }
      />
    </div>
  );
}

function ChatInput({
  message,
  setMessage,
  onSend,
  disabled,
  placeholder,
}: {
  message: string;
  setMessage: (v: string) => void;
  onSend: () => void;
  disabled: boolean;
  placeholder: string;
}) {
  return (
    <div className="border-t border-border p-3 shrink-0">
      <div className="flex gap-2">
        <Input
          placeholder={placeholder}
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          className="flex-1 text-sm"
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              onSend();
            }
          }}
        />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="icon"
              className="h-9 w-9 cursor-pointer shrink-0"
              disabled={disabled || !message.trim()}
              onClick={onSend}
            >
              <IconSend className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Send message</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
