"use client";

import { memo, useEffect, useRef, useState } from "react";
import {
  IconLoader2,
  IconMessageCircle,
  IconPlus,
  IconSend2,
  IconSparkles,
  IconX,
} from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Textarea } from "@kandev/ui/textarea";
import { QuickChatContent } from "@/components/quick-chat/quick-chat-content";
import { useAppStore } from "@/components/state-provider";
import { useConfigChat } from "./use-config-chat";
import type { ConfigChatSession } from "@/lib/state/slices/ui/types";

const SUGGESTION_PROMPTS = [
  "Add a 'Code Review' step to my workflow",
  "Create a new agent profile with auto-approve enabled",
  "Show me the current workflow configuration",
  "Update the MCP servers for the default agent profile",
];

function SuggestionList() {
  return (
    <div className="flex-1 flex flex-col justify-end space-y-1.5 mb-3">
      <p className="text-xs text-muted-foreground font-medium">Try asking</p>
      {SUGGESTION_PROMPTS.map((prompt) => (
        <p key={prompt} className="text-xs text-muted-foreground/70 py-0.5">
          {prompt}
        </p>
      ))}
    </div>
  );
}

function ProfileSelector({ onSelect }: { onSelect: (id: string) => void }) {
  const profiles = useAppStore((s) => s.agentProfiles.items ?? []);
  return (
    <div className="flex-1 flex flex-col">
      <p className="text-xs font-medium mb-3">Select an agent profile</p>
      <div className="space-y-1.5 max-h-[320px] overflow-y-auto">
        {profiles.map((profile) => (
          <button
            key={profile.id}
            type="button"
            onClick={() => onSelect(profile.id)}
            className="w-full flex items-center gap-2.5 rounded-md border p-2.5 text-left transition-colors cursor-pointer hover:border-primary/50 hover:bg-accent/50"
          >
            <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border bg-background">
              <IconMessageCircle className="h-3.5 w-3.5" />
            </div>
            <div className="min-w-0">
              <p className="text-xs font-medium truncate">{profile.label}</p>
              <p className="text-[11px] text-muted-foreground truncate">{profile.agent_name}</p>
            </div>
          </button>
        ))}
      </div>
      <p className="text-[11px] text-muted-foreground mt-3">
        This will be saved as your default. Change it in Agent settings.
      </p>
    </div>
  );
}

function ConfigChatTabs({
  sessions,
  activeSessionId,
  onTabChange,
  onTabClose,
  onNewChat,
}: {
  sessions: ConfigChatSession[];
  activeSessionId: string | null;
  onTabChange: (sessionId: string) => void;
  onTabClose: (sessionId: string) => void;
  onNewChat: () => void;
}) {
  if (sessions.length === 0) return null;

  return (
    <div className="flex items-center gap-1 px-2 py-1 border-b bg-muted/20">
      <div className="flex items-center gap-1 overflow-x-auto flex-1 scrollbar-hide">
        {sessions.map((s, index) => {
          const isActive = s.sessionId === activeSessionId;
          const tabName = s.name || `Chat ${index + 1}`;
          return (
            <div
              key={s.sessionId}
              className={`flex items-center gap-1 rounded transition-colors whitespace-nowrap ${
                isActive
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:bg-muted"
              }`}
            >
              <button
                type="button"
                onClick={() => onTabChange(s.sessionId)}
                className="flex items-center px-2.5 py-1 text-xs cursor-pointer"
              >
                <span className="truncate max-w-[100px]">{tabName}</span>
              </button>
              <button
                type="button"
                aria-label={`Close ${tabName}`}
                className="p-1 cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => onTabClose(s.sessionId)}
              >
                <IconX className="h-3 w-3" />
              </button>
            </div>
          );
        })}
      </div>
      <Button
        size="sm"
        variant="ghost"
        className="h-6 w-6 p-0 cursor-pointer shrink-0"
        onClick={onNewChat}
        aria-label="Start new config chat"
      >
        <IconPlus className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

function LoadingState() {
  return (
    <div className="flex-1 flex flex-col items-center justify-center gap-3">
      <IconLoader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      <p className="text-xs text-muted-foreground">Starting config chat...</p>
    </div>
  );
}

function ConfigChatEmptyState({
  defaultProfileId,
  onSelectPrompt,
  isStarting,
  error,
}: {
  defaultProfileId: string | undefined;
  onSelectPrompt: (prompt: string, profileId: string) => void;
  isStarting: boolean;
  error: string | null;
}) {
  const profileCount = useAppStore((s) => s.agentProfiles.items?.length ?? 0);
  const [selectedProfileId, setSelectedProfileId] = useState(defaultProfileId ?? "");
  const [inputValue, setInputValue] = useState("");

  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const needsProfileSelection = !selectedProfileId && !defaultProfileId && profileCount > 0;
  const effectiveProfileId = selectedProfileId || defaultProfileId || "";
  const canSubmit = inputValue.trim().length > 0 && !!effectiveProfileId && !isStarting;

  useEffect(() => {
    if (!needsProfileSelection && !isStarting && textareaRef.current) {
      textareaRef.current.focus();
    }
  }, [needsProfileSelection, isStarting]);

  const handleSubmit = () => {
    const trimmed = inputValue.trim();
    if (!trimmed || !effectiveProfileId) return;
    onSelectPrompt(trimmed, effectiveProfileId);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  };

  if (isStarting) return <LoadingState />;

  return (
    <div className="flex-1 flex flex-col p-3">
      <div className="text-center space-y-1 mb-3">
        <div className="flex justify-center">
          <div className="flex h-8 w-8 items-center justify-center rounded-full border bg-muted">
            <IconSparkles className="h-4 w-4 text-muted-foreground" />
          </div>
        </div>
        <h3 className="text-sm font-medium">Configure Kandev with AI</h3>
        <p className="text-xs text-muted-foreground">
          Manage workflows, agent profiles, and MCP configuration.
        </p>
      </div>

      {needsProfileSelection ? (
        <ProfileSelector onSelect={setSelectedProfileId} />
      ) : (
        <>
          {inputValue.length === 0 ? <SuggestionList /> : <div className="flex-1" />}

          <div className="flex items-end gap-2">
            <Textarea
              ref={textareaRef}
              value={inputValue}
              onChange={(e) => setInputValue(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Ask anything about your configuration..."
              disabled={!effectiveProfileId || isStarting}
              className="min-h-[40px] max-h-[120px] flex-1 resize-none text-xs"
            />
            <Button
              size="icon"
              onClick={handleSubmit}
              disabled={!canSubmit}
              className="h-[40px] w-[40px] shrink-0 cursor-pointer"
            >
              <IconSend2 className="h-4 w-4" />
            </Button>
          </div>

          {error && <p className="text-xs text-destructive mt-2">{error}</p>}
        </>
      )}

      {profileCount === 0 && (
        <p className="text-xs text-center text-muted-foreground mt-2">
          No agent profiles found. Create one in the Agents settings first.
        </p>
      )}
    </div>
  );
}

type ConfigChatPanelProps = {
  workspaceId: string;
  showFab?: boolean;
};

export const ConfigChatPanel = memo(function ConfigChatPanel({
  workspaceId,
  showFab = true,
}: ConfigChatPanelProps) {
  const chat = useConfigChat(workspaceId);
  const showEmptyState = !chat.activeSessionId;

  return (
    <Popover
      open={chat.isOpen}
      onOpenChange={(nextOpen) => {
        if (nextOpen) {
          chat.open();
        } else {
          chat.close();
        }
      }}
    >
      {showFab ? (
        <Tooltip open={chat.isOpen ? false : undefined}>
          <TooltipTrigger asChild>
            <PopoverTrigger asChild>
              <Button
                size="icon"
                className="fixed bottom-6 right-6 z-50 h-12 w-12 rounded-full shadow-lg cursor-pointer"
              >
                <IconSparkles className="h-8 w-8" />
                <span className="sr-only">Configuration Chat</span>
              </Button>
            </PopoverTrigger>
          </TooltipTrigger>
          <TooltipContent side="left">
            <p className="font-medium">Configuration Chat</p>
            <p className="text-xs text-muted-foreground">Configure Kandev with natural language</p>
          </TooltipContent>
        </Tooltip>
      ) : (
        <PopoverTrigger asChild>
          <span className="fixed bottom-6 right-6 h-0 w-0" aria-hidden />
        </PopoverTrigger>
      )}
      <PopoverContent
        side="top"
        align="end"
        sideOffset={8}
        onInteractOutside={(e) => e.preventDefault()}
        className="w-[420px] max-h-[550px] h-[550px] p-0 gap-0 flex flex-col shadow-2xl"
      >
        <div className="flex items-center justify-between px-3 py-1.5 border-b bg-muted/30">
          <div className="flex items-center gap-1.5">
            <IconSparkles className="h-3.5 w-3.5 text-muted-foreground" />
            <span className="text-xs font-medium">Configuration Chat</span>
          </div>
          <Button
            size="sm"
            variant="ghost"
            className="h-6 w-6 p-0 cursor-pointer"
            onClick={chat.close}
            aria-label="Close config chat"
          >
            <IconX className="h-3.5 w-3.5" />
          </Button>
        </div>
        <ConfigChatTabs
          sessions={chat.sessions}
          activeSessionId={chat.activeSessionId}
          onTabChange={chat.setActiveSession}
          onTabClose={chat.closeSession}
          onNewChat={chat.newChat}
        />
        {showEmptyState ? (
          <ConfigChatEmptyState
            defaultProfileId={chat.defaultProfileId}
            onSelectPrompt={(prompt, profileId) => chat.startSession(profileId, prompt)}
            isStarting={chat.isStarting}
            error={chat.error}
          />
        ) : (
          <QuickChatContent
            sessionId={chat.activeSessionId!}
            minimalToolbar
            placeholderOverride="Ask anything about your configuration..."
            initialPrompt={chat.pendingPrompt ?? undefined}
            onInitialPromptSent={chat.clearPendingPrompt}
          />
        )}
      </PopoverContent>
    </Popover>
  );
});
