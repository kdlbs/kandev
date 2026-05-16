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
import type { Message, TaskSessionState } from "@/lib/types/http";

type AdvancedChatPanelProps = {
  taskId: string;
  sessionId: string | null;
  /** Hide the chat input area (used when embedded in a collapsible panel). */
  hideInput?: boolean;
};

function StartSessionPrompt({
  defaultProfile,
  isLaunching,
  onStart,
}: {
  defaultProfile: { id: string } | null;
  isLaunching: boolean;
  onStart: () => void;
}) {
  return (
    <div className="flex-1 flex flex-col items-center justify-center p-6 text-center">
      <p className="text-sm text-muted-foreground mb-1">No active session for this task.</p>
      <p className="text-xs text-muted-foreground mb-4">
        Start a session or send a message to begin.
      </p>
      {defaultProfile && (
        <Button
          size="sm"
          className="cursor-pointer gap-1.5"
          onClick={onStart}
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
  );
}

function MessageList({
  messages,
  isLoading,
  sessionState,
  taskId,
  sessionId,
  scrollRef,
}: {
  messages: Message[];
  isLoading: boolean;
  sessionState: TaskSessionState | null;
  taskId: string;
  sessionId: string | null;
  scrollRef: React.RefObject<HTMLDivElement | null>;
}) {
  if (isLoading && messages.length === 0) {
    return (
      <div ref={scrollRef} className="flex-1 overflow-y-auto p-4">
        <div className="flex items-center justify-center py-8">
          <IconLoader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      </div>
    );
  }
  return (
    <div ref={scrollRef} className="flex-1 overflow-y-auto p-4">
      <div className="flex flex-col gap-2">
        {messages.map((msg, idx) => (
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
    </div>
  );
}

function useChatActions(
  taskId: string,
  sessionId: string | null,
  defaultProfile: { id: string } | null,
  launch: ReturnType<typeof useSessionLaunch>["launch"],
) {
  const sendNewSession = useCallback(
    async (text: string) => {
      if (!defaultProfile) return;
      const { request } = buildStartRequest(taskId, defaultProfile.id, {
        prompt: text,
        autoStart: true,
      });
      await launch(request);
    },
    [taskId, defaultProfile, launch],
  );

  const sendToExistingSession = useCallback(
    async (text: string) => {
      const client = getWebSocketClient();
      if (!client || !sessionId) return;
      await client.request(
        "message.add",
        { task_id: taskId, session_id: sessionId, content: text },
        10_000,
      );
    },
    [taskId, sessionId],
  );

  return { sendNewSession, sendToExistingSession };
}

export function AdvancedChatPanel({ taskId, sessionId, hideInput }: AdvancedChatPanelProps) {
  const [message, setMessage] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);

  const { session } = useSession(sessionId);
  const { messages, isLoading } = useSessionMessages(sessionId);
  const agentProfiles = useAppStore((s) => s.agentProfiles.items ?? []);
  const defaultProfile = agentProfiles[0] ?? null;

  const { launch, isLoading: isLaunching } = useSessionLaunch();
  const { sendNewSession, sendToExistingSession } = useChatActions(
    taskId,
    sessionId,
    defaultProfile,
    launch,
  );

  const sessionState = session?.state ?? null;
  const isAgentBusy = sessionState === "RUNNING" || sessionState === "STARTING";
  const canSend = sessionId !== null && (sessionState === "WAITING_FOR_INPUT" || isAgentBusy);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages.length]);

  const handleSend = useCallback(async () => {
    const text = message.trim();
    if (!text) return;
    setMessage("");
    if (sessionId) await sendToExistingSession(text);
    else await sendNewSession(text);
  }, [message, sessionId, sendNewSession, sendToExistingSession]);

  const handleStartSession = useCallback(() => sendNewSession(""), [sendNewSession]);

  if (!sessionId && messages.length === 0) {
    return (
      <div className="flex flex-col h-full">
        <StartSessionPrompt
          defaultProfile={defaultProfile}
          isLaunching={isLaunching}
          onStart={handleStartSession}
        />
        {!hideInput && (
          <ChatInput
            message={message}
            setMessage={setMessage}
            onSend={handleSend}
            disabled={!defaultProfile}
            placeholder={
              defaultProfile
                ? "Send a message to start a session..."
                : "No agent profile configured"
            }
          />
        )}
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <MessageList
        messages={messages}
        isLoading={isLoading}
        sessionState={sessionState}
        taskId={taskId}
        sessionId={sessionId}
        scrollRef={scrollRef}
      />
      {!hideInput && (
        <ChatInput
          message={message}
          setMessage={setMessage}
          onSend={handleSend}
          disabled={!canSend && sessionId !== null}
          placeholder={
            isAgentBusy ? "Agent is working... message will be queued" : "Send a message..."
          }
        />
      )}
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
