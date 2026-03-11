"use client";

import { memo, useState } from "react";
import { IconSparkles } from "@tabler/icons-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { QuickChatContent } from "@/components/quick-chat/quick-chat-content";
import { useAppStore } from "@/components/state-provider";
import { useConfigChat } from "./use-config-chat";

const PLACEHOLDER_PROMPTS = [
  "Add a 'Code Review' step to my workflow",
  "Create a new agent profile with auto-approve enabled",
  "Show me the current workflow configuration",
  "Update the MCP servers for the default agent profile",
];

function ConfigChatEmptyState({
  defaultProfileId,
  onSelectPrompt,
  isStarting,
}: {
  defaultProfileId: string | undefined;
  onSelectPrompt: (prompt: string, profileId: string) => void;
  isStarting: boolean;
}) {
  const profiles = useAppStore((s) => s.agentProfiles.items ?? []);
  const [selectedProfileId, setSelectedProfileId] = useState(defaultProfileId ?? "");

  const needsProfileSelection = !defaultProfileId && profiles.length > 0;
  const effectiveProfileId = selectedProfileId || defaultProfileId || "";

  const handlePromptClick = (prompt: string) => {
    if (!effectiveProfileId) return;
    onSelectPrompt(prompt, effectiveProfileId);
  };

  return (
    <div className="flex-1 flex flex-col items-center justify-center p-4">
      <div className="w-full space-y-4">
        <div className="text-center space-y-1">
          <div className="flex justify-center">
            <div className="flex h-10 w-10 items-center justify-center rounded-full border bg-muted">
              <IconSparkles className="h-5 w-5 text-muted-foreground" />
            </div>
          </div>
          <h3 className="text-sm font-medium">Configure Kandev with AI</h3>
          <p className="text-xs text-muted-foreground">
            Manage workflows, agent profiles, and MCP configuration.
          </p>
        </div>

        {needsProfileSelection && (
          <div className="space-y-1.5">
            <label className="text-xs font-medium" htmlFor="config-agent-select">
              Select an agent profile
            </label>
            <Select value={selectedProfileId} onValueChange={setSelectedProfileId}>
              <SelectTrigger id="config-agent-select" className="w-full cursor-pointer">
                <SelectValue placeholder="Choose an agent profile..." />
              </SelectTrigger>
              <SelectContent>
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
        )}

        <div className="grid grid-cols-1 gap-2">
          {PLACEHOLDER_PROMPTS.map((prompt) => (
            <button
              key={prompt}
              onClick={() => handlePromptClick(prompt)}
              disabled={!effectiveProfileId || isStarting}
              className="flex items-start rounded-md border p-3 text-left transition-all hover:border-primary hover:bg-accent cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <p className="text-xs">{prompt}</p>
            </button>
          ))}
        </div>

        {profiles.length === 0 && (
          <p className="text-xs text-center text-muted-foreground">
            No agent profiles found. Create one in the Agents settings first.
          </p>
        )}
      </div>
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
