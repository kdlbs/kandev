"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconLoader2, IconMessageCircle, IconSend2, IconSparkles } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { isSelectableAgentProfile } from "@/lib/state/slices/settings/types";
import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";
import { ConfigurationChatToggle } from "@/components/quick-chat/configuration-chat-toggle";
import { orderAgentProfilesByRecentUse } from "@/lib/agent-profile-recent-use";

/**
 * Catalog KEYS, not copy. Resolving these at module scope would freeze them at
 * the boot locale (and hide them from the pseudo-locale, which only ever sees
 * text that was never translated) — see docs/i18n.md. `Suggestions` resolves
 * each one at render.
 *
 * These are display copy that the user then SENDS: picking one fills the
 * composer, so the agent receives whatever locale the user is reading in. That
 * is the intent — a French user should be able to send a French prompt — and
 * none of these values is compared or persisted.
 */
const SUGGESTION_PROMPT_KEYS = [
  "configChat:suggestionAddReviewStep",
  "configChat:suggestionCreateProfile",
  "configChat:suggestionShowWorkflow",
  "configChat:suggestionUpdateMcp",
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
  const profiles = useAppStore((state) => state.agentProfiles.items ?? []);
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  const { t } = useTranslation();
  const recentProfileIds = useAppStore(
    (state) => state.agentProfileRecentUse?.records.config_chat?.profileIds,
  );
  const selectableProfiles = profiles.filter((profile) =>
    isSelectableAgentProfile(profile, dynamicRoutingEnabled),
  );
  const orderedProfiles = orderAgentProfilesByRecentUse(selectableProfiles, recentProfileIds);
  return (
    <section className="space-y-3" aria-labelledby="config-chat-agent-label">
      <div>
        <h3 id="config-chat-agent-label" className="text-sm font-medium">
          {t("configChat:agentProfileHeading")}
        </h3>
        <p className="text-xs text-muted-foreground">{t("configChat:agentProfileHelp")}</p>
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        {orderedProfiles.map((profile) => (
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
  const { t } = useTranslation();
  return (
    <section className="space-y-2" aria-labelledby="config-chat-suggestions-label">
      <h3 id="config-chat-suggestions-label" className="text-xs font-medium text-muted-foreground">
        {t("configChat:tryAsking")}
      </h3>
      <div className="grid gap-2 sm:grid-cols-2">
        {/* Keyed by the CATALOG KEY, not the resolved prompt: a React key should
            be a stable identity, and keying on translated copy remounts every
            button on a locale switch. */}
        {SUGGESTION_PROMPT_KEYS.map((key) => (
          <button
            key={key}
            type="button"
            onClick={() => onSelect(t(key))}
            className="min-h-11 rounded-md border px-3 py-2 text-left text-xs text-muted-foreground transition-colors hover:border-primary/50 hover:bg-accent/50 hover:text-foreground"
          >
            {t(key)}
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
  const { t } = useTranslation();
  return (
    <footer className="flex shrink-0 items-center justify-end gap-2 border-t bg-popover px-4 py-3 sm:px-8">
      <Button variant="outline" onClick={onCancel} disabled={disabled} className="cursor-pointer">
        {t("common:cancel")}
      </Button>
      <Button
        onClick={onStart}
        disabled={startDisabled}
        className="min-w-28 cursor-pointer"
        aria-label={t("configChat:startChatAria")}
        data-dialog-default-action
      >
        {isStarting ? <IconLoader2 className="h-4 w-4 animate-spin" aria-hidden /> : null}
        {isStarting ? t("configChat:startingChat") : t("configChat:startChat")}
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
  const { t } = useTranslation();
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
          {t("configChat:promptHeading")}
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
            placeholder={t("configChat:promptPlaceholder")}
            disabled={disabled}
            className="min-h-20 max-h-32 resize-y"
          />
          {presentation === "floating" && (
            <Button
              size="icon"
              onClick={onSubmit}
              disabled={!canSubmit}
              className="h-11 w-11 shrink-0 cursor-pointer"
              aria-label={t("configChat:startChatAria")}
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
  const { t } = useTranslation();
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
                <h2 className="text-lg font-semibold">{t("common:configurationChat")}</h2>
              </div>
              <p className="text-sm text-muted-foreground">{t("configChat:dialogDescription")}</p>
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
          <p className="text-sm text-muted-foreground">{t("configChat:noAgentProfiles")}</p>
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
  const profiles = useAppStore((state) => state.agentProfiles.items ?? []);
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  const selectableProfiles = profiles.filter((profile) =>
    isSelectableAgentProfile(profile, dynamicRoutingEnabled),
  );
  const [selectedProfileId, setSelectedProfileId] = useState(defaultProfileId ?? "");
  const [inputValue, setInputValue] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    if (!defaultProfileId) return;
    setSelectedProfileId((current) => current || defaultProfileId);
  }, [defaultProfileId]);

  const effectiveProfileId = selectedProfileId || defaultProfileId || "";
  const profileIsResolved = selectableProfiles.some((profile) => profile.id === effectiveProfileId);
  const needsProfileSelection = selectableProfiles.length > 0 && !profileIsResolved;
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
        hasProfiles={selectableProfiles.length > 0}
        needsProfileSelection={needsProfileSelection}
        isStarting={isStarting}
        onSelectProfile={setSelectedProfileId}
        onSelectSuggestion={setInputValue}
        onKindChange={onKindChange}
      />
      <ConfigSetupActions
        showPrompt={selectableProfiles.length > 0 && !needsProfileSelection}
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
