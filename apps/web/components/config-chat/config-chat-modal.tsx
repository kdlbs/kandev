"use client";

import { memo } from "react";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { IconSparkles } from "@tabler/icons-react";
import { QuickChatContent } from "@/components/quick-chat/quick-chat-content";
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

function ConfigChatEmptyState({ onSelectPrompt }: { onSelectPrompt: (prompt: string) => void }) {
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
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {PLACEHOLDER_PROMPTS.map((prompt) => (
            <button
              key={prompt}
              onClick={() => onSelectPrompt(prompt)}
              className="group relative flex items-start gap-3 rounded-lg border p-4 text-left transition-all hover:border-primary hover:bg-accent cursor-pointer"
            >
              <p className="text-sm">{prompt}</p>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

export const ConfigChatModal = memo(function ConfigChatModal({
  workspaceId,
}: ConfigChatModalProps) {
  const { isOpen, sessionId, close } = useConfigChat(workspaceId);

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
          <ConfigChatEmptyState onSelectPrompt={() => {}} />
        )}
      </DialogContent>
    </Dialog>
  );
});
