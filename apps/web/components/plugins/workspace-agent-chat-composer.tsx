"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import type { PluginManagedConversationInputIntent } from "@kandev/plugin-sdk";
import { createManagedConversationInputIntent } from "@/lib/plugins/managed-conversation";

export type WorkspaceAgentChatComposerState = {
  draft: string;
  intent: PluginManagedConversationInputIntent | null;
  submitting: boolean;
  failed: boolean;
};

function submitLabel(submitting: boolean, failed: boolean, t: (key: string) => string): string {
  if (submitting) return t("managedChatSending");
  if (failed) return t("managedChatRetry");
  return t("managedChatSend");
}

export const EMPTY_WORKSPACE_AGENT_CHAT_COMPOSER_STATE: WorkspaceAgentChatComposerState = {
  draft: "",
  intent: null,
  submitting: false,
  failed: false,
};

export function WorkspaceAgentChatComposer({
  enabled,
  touchTargets,
  generateKey,
  onSubmit,
  state,
  updateState,
}: {
  enabled: boolean;
  touchTargets: boolean;
  generateKey(): string;
  onSubmit(intent: PluginManagedConversationInputIntent): Promise<void>;
  state: WorkspaceAgentChatComposerState;
  updateState(patch: Partial<WorkspaceAgentChatComposerState>): void;
}) {
  const { t } = useTranslation("plugins");

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!enabled || state.submitting) return;
    const payload = state.draft.trim();
    let intent = state.intent;
    if (!intent) {
      if (!payload) return;
      intent = createManagedConversationInputIntent(payload, generateKey());
    }
    updateState({ intent, submitting: true, failed: false });
    try {
      await onSubmit(intent);
      updateState({ intent: null, draft: "", failed: false });
    } catch {
      updateState({ failed: true });
    } finally {
      updateState({ submitting: false });
    }
  }

  return (
    <form
      data-testid="managed-chat-composer"
      className="flex shrink-0 items-end gap-2 border-t bg-background p-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]"
      onSubmit={submit}
    >
      <Textarea
        aria-label={t("managedChatMessageLabel")}
        placeholder={t("managedChatMessagePlaceholder")}
        className={`${touchTargets ? "min-h-11" : "min-h-7 py-0.5"} max-h-40 resize-y text-base md:text-sm`}
        value={state.draft}
        disabled={!enabled || state.submitting || state.intent !== null}
        onChange={(event) => updateState({ draft: event.target.value })}
      />
      <Button
        type="submit"
        className={`${touchTargets ? "min-h-11" : "min-h-7"} min-w-16 shrink-0`}
        disabled={!enabled || state.submitting || (!state.draft.trim() && !state.intent)}
      >
        {submitLabel(state.submitting, state.failed, t)}
      </Button>
      {state.failed ? (
        <span role="status" className="sr-only">
          {t("managedChatInputRetryAvailable")}
        </span>
      ) : null}
    </form>
  );
}
