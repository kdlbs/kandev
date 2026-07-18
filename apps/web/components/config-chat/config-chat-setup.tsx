"use client";

import { useEffect, useRef, useState } from "react";
import { IconLoader2, IconMessageCircle, IconSend2, IconSparkles } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";
import { ConfigurationChatToggle } from "@/components/quick-chat/configuration-chat-toggle";

const SUGGESTION_PROMPTS = [
  "Add a 'Code Review' step to my workflow",
  "Create a new agent profile with auto-approve enabled",
  "Show me the current workflow configuration",
  "Update the MCP servers for the default agent profile",
];

type ConfigChatSetupBaseProps = {
  defaultProfileId?: string;
  isStarting: boolean;
  error: string | null;
  onStart: (profileId: string, prompt: string) => void;
  onKindChange?: (kind: QuickChatSessionKind) => void;
};

type ConfigChatSetupProps = ConfigChatSetupBaseProps &
  (
    | { presentation: "floating"; onCancel?: never }
    | { presentation?: "dialog"; onCancel: () => void }
  );

function ProfileSelector({ onSelect }: { onSelect: (id: string) => void }) {
  const { agentProfiles: profiles } = useSettingsData(true);
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

function ConfigChatFooter({
  onCancel,
  onStart,
  disabled,
  startDisabled,
  isStarting,
}: {
  onCancel: () => void;
  onStart: () => void;
  disabled: boolean;
  startDisabled: boolean;
  isStarting: boolean;
}) {
  return (
    <footer className="flex shrink-0 items-center justify-end gap-2 border-t bg-popover px-4 py-3 sm:px-8">
      <Button variant="outline" onClick={onCancel} disabled={disabled} className="cursor-pointer">
        Cancel
      </Button>
      <Button
        onClick={onStart}
        disabled={startDisabled}
        className="min-w-28 cursor-pointer"
        aria-label="Start configuration chat"
        data-dialog-default-action
      >
        {isStarting ? <IconLoader2 className="h-4 w-4 animate-spin" aria-hidden /> : null}
        {isStarting ? "Starting chat..." : "Start chat"}
      </Button>
    </footer>
  );
}

type ConfigPromptProps = {
  value: string;
  error: string | null;
  disabled: boolean;
  canSubmit: boolean;
  isStarting: boolean;
  presentation: "dialog" | "floating";
  textareaRef: React.RefObject<HTMLTextAreaElement | null>;
  onChange: (value: string) => void;
  onSubmit: () => void;
};

function ConfigPrompt({
  value,
  error,
  disabled,
  canSubmit,
  isStarting,
  presentation,
  textareaRef,
  onChange,
  onSubmit,
}: ConfigPromptProps) {
  return (
    <div
      className="shrink-0 border-t bg-popover px-4 py-4 sm:px-8"
      data-testid="config-chat-composer"
    >
      <section
        className="mx-auto w-full max-w-2xl space-y-2"
        aria-labelledby="config-chat-prompt-label"
      >
        <h3 id="config-chat-prompt-label" className="text-sm font-medium">
          What would you like to configure?
        </h3>
        <div className="flex items-end gap-2">
          <Textarea
            ref={textareaRef}
            value={value}
            onChange={(event) => onChange(event.target.value)}
            onKeyDown={(event) => {
              if (
                event.key === "Enter" &&
                !event.shiftKey &&
                !event.repeat &&
                !event.nativeEvent.isComposing &&
                event.keyCode !== 229
              ) {
                event.preventDefault();
                onSubmit();
              }
            }}
            placeholder="Ask anything about your configuration..."
            disabled={disabled}
            className="min-h-20 max-h-32 resize-y"
          />
          {presentation === "floating" && (
            <Button
              size="icon"
              onClick={onSubmit}
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
          )}
        </div>
        {error && <p className="text-sm text-destructive">{error}</p>}
      </section>
    </div>
  );
}

function ConfigGuidance({
  presentation,
  hasProfiles,
  needsProfileSelection,
  isStarting,
  onSelectProfile,
  onSelectSuggestion,
  onKindChange,
}: {
  presentation: "dialog" | "floating";
  hasProfiles: boolean;
  needsProfileSelection: boolean;
  isStarting: boolean;
  onSelectProfile: (profileId: string) => void;
  onSelectSuggestion: (prompt: string) => void;
  onKindChange?: (kind: QuickChatSessionKind) => void;
}) {
  return (
    <div
      className="min-h-0 flex-1 overflow-y-auto px-4 py-6 sm:px-8 sm:py-8"
      data-testid="config-chat-guidance"
    >
      <div className="mx-auto w-full max-w-2xl space-y-7">
        {presentation === "dialog" && (
          <>
            <header className="space-y-2">
              <div className="flex items-center gap-2">
                <IconSparkles className="h-5 w-5 text-primary" aria-hidden />
                <h2 className="text-lg font-semibold">Configuration Chat</h2>
              </div>
              <p className="text-sm text-muted-foreground">
                Ask an agent to manage workflows, agent profiles, and MCP configuration.
              </p>
            </header>
            {onKindChange && (
              <ConfigurationChatToggle
                checked
                disabled={isStarting}
                onCheckedChange={(checked) => !checked && onKindChange("chat")}
              />
            )}
          </>
        )}

        {!hasProfiles && (
          <p className="text-sm text-muted-foreground">
            No agent profiles are available. Create one in Agent settings first.
          </p>
        )}

        {hasProfiles && needsProfileSelection && <ProfileSelector onSelect={onSelectProfile} />}

        {hasProfiles && !needsProfileSelection && <Suggestions onSelect={onSelectSuggestion} />}
      </div>
    </div>
  );
}

function ConfigSetupActions({
  showPrompt,
  value,
  error,
  profileIsResolved,
  canSubmit,
  isStarting,
  presentation,
  onCancel,
  textareaRef,
  onChange,
  onSubmit,
}: {
  showPrompt: boolean;
  value: string;
  error: string | null;
  profileIsResolved: boolean;
  canSubmit: boolean;
  isStarting: boolean;
  presentation: "dialog" | "floating";
  onCancel?: () => void;
  textareaRef: React.RefObject<HTMLTextAreaElement | null>;
  onChange: (value: string) => void;
  onSubmit: () => void;
}) {
  return (
    <>
      {showPrompt && (
        <ConfigPrompt
          value={value}
          error={error}
          disabled={!profileIsResolved || isStarting}
          canSubmit={canSubmit}
          isStarting={isStarting}
          presentation={presentation}
          textareaRef={textareaRef}
          onChange={onChange}
          onSubmit={onSubmit}
        />
      )}
      {presentation === "dialog" && onCancel && (
        <ConfigChatFooter
          onCancel={onCancel}
          onStart={onSubmit}
          disabled={isStarting}
          startDisabled={!canSubmit}
          isStarting={isStarting}
        />
      )}
    </>
  );
}

export function ConfigChatSetup({
  presentation = "dialog",
  defaultProfileId,
  isStarting,
  error,
  onStart,
  onCancel,
  onKindChange,
}: ConfigChatSetupProps) {
  const { agentProfiles: profiles } = useSettingsData(true);
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
    <div className="flex min-h-0 flex-1 flex-col bg-popover" data-testid="config-chat-setup">
      <ConfigGuidance
        presentation={presentation}
        hasProfiles={profiles.length > 0}
        needsProfileSelection={needsProfileSelection}
        isStarting={isStarting}
        onSelectProfile={setSelectedProfileId}
        onSelectSuggestion={setInputValue}
        onKindChange={onKindChange}
      />
      <ConfigSetupActions
        showPrompt={profiles.length > 0 && !needsProfileSelection}
        value={inputValue}
        error={error}
        profileIsResolved={profileIsResolved}
        canSubmit={canSubmit}
        isStarting={isStarting}
        presentation={presentation}
        onCancel={onCancel}
        textareaRef={textareaRef}
        onChange={setInputValue}
        onSubmit={handleSubmit}
      />
    </div>
  );
}
