"use client";

import { memo, useState } from "react";
import { IconLoader2, IconSend2, IconSparkles } from "@tabler/icons-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Textarea } from "@kandev/ui/textarea";
import { QuickChatContent } from "@/components/quick-chat/quick-chat-content";
import { useAppStore } from "@/components/state-provider";
import { useConfigChat } from "./use-config-chat";

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

function ProfileSelector({
  selectedId,
  onSelect,
}: {
  selectedId: string;
  onSelect: (id: string) => void;
}) {
  const profiles = useAppStore((s) => s.agentProfiles.items ?? []);
  return (
    <div className="space-y-2.5 mb-3">
      <label className="text-xs font-medium" htmlFor="config-agent-select">
        Select an agent profile
      </label>
      <Select value={selectedId} onValueChange={onSelect}>
        <SelectTrigger id="config-agent-select" className="w-full cursor-pointer">
          <SelectValue placeholder="Choose an agent profile..." />
        </SelectTrigger>
        <SelectContent className="w-[var(--radix-select-trigger-width)]">
          {profiles.map((profile) => (
            <SelectItem key={profile.id} value={profile.id} className="cursor-pointer">
              {profile.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">
        This will be saved as your default. Change it in Agent settings.
      </p>
    </div>
  );
}

function ConfigChatEmptyState({
  defaultProfileId,
  onSelectPrompt,
  isStarting,
}: {
  defaultProfileId: string | undefined;
  onSelectPrompt: (prompt: string, profileId: string) => void;
  isStarting: boolean;
}) {
  const profileCount = useAppStore((s) => s.agentProfiles.items?.length ?? 0);
  const [selectedProfileId, setSelectedProfileId] = useState(defaultProfileId ?? "");
  const [inputValue, setInputValue] = useState("");

  const needsProfileSelection = !defaultProfileId && profileCount > 0;
  const effectiveProfileId = selectedProfileId || defaultProfileId || "";
  const canSubmit = inputValue.trim().length > 0 && !!effectiveProfileId && !isStarting;

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

      {needsProfileSelection && (
        <ProfileSelector selectedId={selectedProfileId} onSelect={setSelectedProfileId} />
      )}

      {inputValue.length === 0 ? (
        <SuggestionList />
      ) : (
        <div className="flex-1" />
      )}

      <div className="flex items-end gap-2">
        <Textarea
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
          {isStarting ? (
            <IconLoader2 className="h-4 w-4 animate-spin" />
          ) : (
            <IconSend2 className="h-4 w-4" />
          )}
        </Button>
      </div>

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
};

export const ConfigChatPanel = memo(function ConfigChatPanel({
  workspaceId,
}: ConfigChatPanelProps) {
  const { isOpen, sessionId, isStarting, defaultProfileId, startSession, open, close } =
    useConfigChat(workspaceId);

  return (
    <Popover
      open={isOpen}
      onOpenChange={(nextOpen) => {
        if (nextOpen) {
          open();
        } else {
          close();
        }
      }}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              size="icon"
              className="fixed bottom-6 right-6 z-50 h-12 w-12 rounded-full shadow-lg cursor-pointer"
            >
              <IconSparkles className="h-7 w-7" />
              <span className="sr-only">Configuration Chat</span>
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent side="left">
          <p className="font-medium">Configuration Chat</p>
          <p className="text-xs text-muted-foreground">Configure Kandev with natural language</p>
        </TooltipContent>
      </Tooltip>
      <PopoverContent
        side="top"
        align="end"
        sideOffset={8}
        className="w-[420px] max-h-[550px] h-[550px] p-0 gap-0 flex flex-col shadow-2xl"
      >
        {sessionId ? (
          <QuickChatContent sessionId={sessionId} minimalToolbar />
        ) : (
          <ConfigChatEmptyState
            defaultProfileId={defaultProfileId}
            onSelectPrompt={(prompt, profileId) => startSession(profileId, prompt)}
            isStarting={isStarting}
          />
        )}
      </PopoverContent>
    </Popover>
  );
});
