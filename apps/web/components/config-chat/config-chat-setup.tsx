"use client";

import { useEffect, useRef, useState } from "react";
import { IconLoader2, IconMessageCircle, IconSend2, IconSparkles } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { useAppStore } from "@/components/state-provider";

const SUGGESTION_PROMPTS = [
  "Add a 'Code Review' step to my workflow",
  "Create a new agent profile with auto-approve enabled",
  "Show me the current workflow configuration",
  "Update the MCP servers for the default agent profile",
];

type ConfigChatSetupProps = {
  defaultProfileId?: string;
  isStarting: boolean;
  error: string | null;
  onStart: (profileId: string, prompt: string) => void;
  onCancel: () => void;
};

function ProfileSelector({ onSelect }: { onSelect: (id: string) => void }) {
  const profiles = useAppStore((state) => state.agentProfiles.items ?? []);
  return (
    <section className="space-y-3" aria-labelledby="config-chat-agent-label">
      <div>
        <h3 id="config-chat-agent-label" className="text-sm font-medium">
          Configuration agent profile
        </h3>
        <p className="text-xs text-muted-foreground">
          Choose the agent with access to configuration tools. This becomes the workspace default.
        </p>
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        {profiles.map((profile) => (
          <button
            key={profile.id}
            type="button"
            onClick={() => onSelect(profile.id)}
            className="flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-md border p-3 text-left transition-colors hover:border-primary/50 hover:bg-accent/50"
          >
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border bg-background">
              <IconMessageCircle className="h-4 w-4" aria-hidden />
            </span>
            <span className="min-w-0">
              <span className="block truncate text-sm font-medium">{profile.label}</span>
              <span className="block truncate text-xs text-muted-foreground">
                {profile.agent_name}
              </span>
            </span>
          </button>
        ))}
      </div>
    </section>
  );
}

function Suggestions({ onSelect }: { onSelect: (prompt: string) => void }) {
  return (
    <section className="space-y-2" aria-labelledby="config-chat-suggestions-label">
      <h3 id="config-chat-suggestions-label" className="text-xs font-medium text-muted-foreground">
        Try asking
      </h3>
      <div className="grid gap-2 sm:grid-cols-2">
        {SUGGESTION_PROMPTS.map((prompt) => (
          <button
            key={prompt}
            type="button"
            onClick={() => onSelect(prompt)}
            className="min-h-11 rounded-md border px-3 py-2 text-left text-xs text-muted-foreground transition-colors hover:border-primary/50 hover:bg-accent/50 hover:text-foreground"
          >
            {prompt}
          </button>
        ))}
      </div>
    </section>
  );
}

function ConfigChatFooter({ onCancel, disabled }: { onCancel: () => void; disabled: boolean }) {
  return (
    <footer className="flex shrink-0 justify-end border-t bg-background px-4 py-3 sm:px-8">
      <Button variant="outline" onClick={onCancel} disabled={disabled} className="cursor-pointer">
        Cancel
      </Button>
    </footer>
  );
}

export function ConfigChatSetup({
  defaultProfileId,
  isStarting,
  error,
  onStart,
  onCancel,
}: ConfigChatSetupProps) {
  const profiles = useAppStore((state) => state.agentProfiles.items ?? []);
  const [selectedProfileId, setSelectedProfileId] = useState(defaultProfileId ?? "");
  const [inputValue, setInputValue] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    if (!defaultProfileId) return;
    setSelectedProfileId((current) => current || defaultProfileId);
  }, [defaultProfileId]);

  const effectiveProfileId = selectedProfileId || defaultProfileId || "";
  const profileIsResolved = profiles.some((profile) => profile.id === effectiveProfileId);
  const needsProfileSelection = profiles.length > 0 && !profileIsResolved;
  const canSubmit = inputValue.trim().length > 0 && profileIsResolved && !isStarting;

  useEffect(() => {
    if (!needsProfileSelection && !isStarting) textareaRef.current?.focus();
  }, [isStarting, needsProfileSelection]);

  const handleSubmit = () => {
    const prompt = inputValue.trim();
    if (!prompt || !effectiveProfileId || isStarting) return;
    onStart(effectiveProfileId, prompt);
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="config-chat-setup">
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-6 sm:px-8 sm:py-8">
        <div className="mx-auto w-full max-w-2xl space-y-7">
          <header className="space-y-2">
            <div className="flex items-center gap-2">
              <IconSparkles className="h-5 w-5 text-primary" aria-hidden />
              <h2 className="text-lg font-semibold">Configuration Chat</h2>
            </div>
            <p className="text-sm text-muted-foreground">
              Ask an agent to manage workflows, agent profiles, and MCP configuration.
            </p>
          </header>

          {profiles.length === 0 && (
            <p className="text-sm text-muted-foreground">
              No agent profiles are available. Create one in Agent settings first.
            </p>
          )}

          {profiles.length > 0 && needsProfileSelection && (
            <ProfileSelector onSelect={setSelectedProfileId} />
          )}

          {profiles.length > 0 && !needsProfileSelection && (
            <>
              <Suggestions onSelect={(prompt) => setInputValue(prompt)} />
              <section className="space-y-2" aria-labelledby="config-chat-prompt-label">
                <h3 id="config-chat-prompt-label" className="text-sm font-medium">
                  What would you like to configure?
                </h3>
                <div className="flex items-end gap-2">
                  <Textarea
                    ref={textareaRef}
                    value={inputValue}
                    onChange={(event) => setInputValue(event.target.value)}
                    onKeyDown={(event) => {
                      if (
                        event.key === "Enter" &&
                        !event.shiftKey &&
                        !event.repeat &&
                        !event.nativeEvent.isComposing &&
                        event.keyCode !== 229
                      ) {
                        event.preventDefault();
                        handleSubmit();
                      }
                    }}
                    placeholder="Ask anything about your configuration..."
                    disabled={!profileIsResolved || isStarting}
                    className="min-h-24 max-h-48 flex-1 resize-y"
                  />
                  <Button
                    size="icon"
                    onClick={handleSubmit}
                    disabled={!canSubmit}
                    className="h-11 w-11 shrink-0 cursor-pointer"
                    aria-label="Start configuration chat"
                  >
                    {isStarting ? (
                      <IconLoader2 className="h-4 w-4 animate-spin" aria-hidden />
                    ) : (
                      <IconSend2 className="h-4 w-4" aria-hidden />
                    )}
                  </Button>
                </div>
                {error && <p className="text-sm text-destructive">{error}</p>}
              </section>
            </>
          )}
        </div>
      </div>
      <ConfigChatFooter onCancel={onCancel} disabled={isStarting} />
    </div>
  );
}
