"use client";

import { memo, useState } from "react";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { IconSparkles } from "@tabler/icons-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { QuickChatContent } from "@/components/quick-chat/quick-chat-content";
import { useAppStore } from "@/components/state-provider";
import { useConfigChat } from "./use-config-chat";

type ConfigChatModalProps = {
  workspaceId: string;
};

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
    <div className="flex-1 flex flex-col items-center justify-center p-8">
      <div className="max-w-2xl w-full space-y-6">
        <div className="text-center space-y-2">
          <div className="flex justify-center">
            <div className="flex h-12 w-12 items-center justify-center rounded-full border bg-muted">
              <IconSparkles className="h-6 w-6 text-muted-foreground" />
            </div>
          </div>
          <h3 className="text-lg font-medium">Configure Kandev with AI</h3>
          <p className="text-sm text-muted-foreground">
            Chat with an agent to manage your workflows, agent profiles, and MCP server
            configuration.
          </p>
        </div>

        {needsProfileSelection && (
          <div className="space-y-2">
            <label className="text-sm font-medium" htmlFor="config-agent-select">
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
              This will be saved as your default config agent. You can change it in workspace
              settings.
            </p>
          </div>
        )}

        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {PLACEHOLDER_PROMPTS.map((prompt) => (
            <button
              key={prompt}
              onClick={() => handlePromptClick(prompt)}
              disabled={!effectiveProfileId || isStarting}
              className="group relative flex items-start gap-3 rounded-lg border p-4 text-left transition-all hover:border-primary hover:bg-accent cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <p className="text-sm">{prompt}</p>
            </button>
          ))}
        </div>

        {profiles.length === 0 && (
          <p className="text-sm text-center text-muted-foreground">
            No agent profiles found. Create one in the Agents settings first.
          </p>
        )}
      </div>
    </div>
  );
}

export const ConfigChatModal = memo(function ConfigChatModal({
  workspaceId,
}: ConfigChatModalProps) {
  const { isOpen, sessionId, isStarting, defaultProfileId, startSession, close } =
    useConfigChat(workspaceId);

  return (
    <Dialog open={isOpen} onOpenChange={(open) => !open && close()}>
      <DialogContent
        className="!max-w-[80vw] !w-[80vw] max-h-[85vh] h-[85vh] p-0 gap-0 flex flex-col shadow-2xl"
        showCloseButton={false}
        overlayClassName="bg-transparent"
      >
        <DialogTitle className="sr-only">Config Chat</DialogTitle>
        <div className="flex items-center gap-2 px-4 py-2 border-b bg-muted/20">
          <IconSparkles className="h-4 w-4 text-muted-foreground" />
          <span className="text-sm font-medium">Config Chat</span>
        </div>
        {sessionId ? (
          <QuickChatContent sessionId={sessionId} />
        ) : (
          <ConfigChatEmptyState
            defaultProfileId={defaultProfileId}
            onSelectPrompt={(prompt, profileId) => startSession(profileId, prompt)}
            isStarting={isStarting}
          />
        )}
      </DialogContent>
    </Dialog>
  );
});
