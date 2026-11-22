"use client";

import { useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { PromptListItem } from "@/components/settings/prompt-list-item";
import { PromptSyncSection } from "@/components/settings/prompt-sync-section";
import { PromptConflictNotice } from "@/components/settings/prompt-conflict-review-dialog";
import { PromptOrphanedDraftsNotice } from "@/components/settings/prompt-orphaned-drafts-notice";
import {
  filterSavedPrompts,
  PromptCreateButton,
  PromptListSearch,
} from "@/components/settings/prompt-list-search";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { SettingsGroup } from "@/components/settings/settings-group";
import { SettingsPromptEditor } from "@/components/settings/settings-prompt-editor";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  choiceFor,
  draftForm,
  isLinkedPrompt,
  isSwitched,
  orphanedDrafts,
  type PromptFormState,
} from "@/lib/settings/prompt-versions";
import type { PromptConflictChoice } from "@/lib/types/prompt-sync";
import type { CustomPrompt } from "@/lib/types/http";
import {
  activeRowIds,
  emptyPromptForm,
  getPromptDraftMeta,
  usePromptsActions,
  usePromptsState,
  type PromptsState,
} from "./use-prompts-page";

/** The sigil the chat input matches on. Typed verbatim, so never translated. */
const PROMPT_MENTION_TOKEN = "@name";

type PromptCreateFormProps = {
  formState: PromptFormState;
  onFormChange: (patch: Partial<PromptFormState>) => void;
  onCancel: () => void;
  isBusy: boolean;
};

function PromptCreateForm({ formState, onFormChange, onCancel, isBusy }: PromptCreateFormProps) {
  const { t } = useTranslation();
  const nameIsDirty = formState.name !== emptyPromptForm.name;
  const contentIsDirty = formState.content !== emptyPromptForm.content;
  return (
    <div
      className="rounded-lg border border-border/70 bg-background p-4 space-y-3"
      data-testid="prompt-create-form"
      data-settings-dirty="true"
      data-settings-dirty-level="container"
    >
      <div className="text-sm font-medium text-foreground">{t("settings:promptAdd")}</div>
      <Input
        value={formState.name}
        onChange={(event) => onFormChange({ name: event.target.value })}
        placeholder={t("settings:promptNamePlaceholder")}
        data-testid="prompt-name-input"
        disabled={isBusy}
        data-settings-dirty={nameIsDirty}
      />
      <SettingsPromptEditor
        value={formState.content}
        onChange={(value) => onFormChange({ content: value })}
        promptReferences
        readOnly={isBusy}
        testId="prompt-content-input"
        isDirty={contentIsDirty}
        dirtyLevel="field"
      />
      <div className="flex items-center gap-2">
        <Button variant="ghost" onClick={onCancel} disabled={isBusy}>
          {t("settings:cancel")}
        </Button>
      </div>
    </div>
  );
}

type PromptRowsProps = {
  state: PromptsState;
  actions: ReturnType<typeof usePromptsActions>;
  prompts: CustomPrompt[];
  isFinePointer: boolean;
};

// PromptRows renders the listed prompts with their open state, unsaved edit
// and pending version switch.
function PromptRows({ state, actions, prompts, isFinePointer }: PromptRowsProps) {
  const { openRows, drafts, pending, deleteTarget, showCreate } = state;
  return (
    <>
      {prompts.map((prompt: CustomPrompt) => (
        <PromptListItem
          key={prompt.id}
          prompt={prompt}
          version={
            isLinkedPrompt(prompt)
              ? {
                  choice: choiceFor(prompt, pending),
                  pending: isSwitched(prompt, pending),
                  onChoose: (choice: PromptConflictChoice) => actions.choose(prompt, choice),
                }
              : undefined
          }
          open={Boolean(openRows[prompt.id])}
          onToggleOpen={actions.toggleOpen}
          form={draftForm(prompt, drafts)}
          edited={drafts[prompt.id] !== undefined}
          onFormChange={actions.editForm}
          onCancel={actions.cancelEdit}
          onOpenDelete={actions.openDeleteDialog}
          onDeleteCancel={actions.closeDeleteDialog}
          onDeleteConfirm={actions.confirmDelete}
          isBusy={actions.isBusy}
          showCreate={showCreate}
          isFinePointer={isFinePointer}
          isDeleteTarget={deleteTarget?.id === prompt.id}
        />
      ))}
    </>
  );
}

function PromptListMessage({ children }: { children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
      {children}
    </div>
  );
}

function PromptListContent(
  props: PromptRowsProps & { promptsLoaded: boolean; hasPrompts: boolean },
) {
  const { t } = useTranslation();
  if (!props.promptsLoaded)
    return <PromptListMessage>{t("settings:promptsLoading")}</PromptListMessage>;
  if (!props.hasPrompts) return <PromptListMessage>{t("settings:promptsEmpty")}</PromptListMessage>;
  if (props.prompts.length === 0)
    return <PromptListMessage>{t("common:noResults")}</PromptListMessage>;
  return <PromptRows {...props} />;
}

function PromptMentionHelp() {
  return (
    <div className="rounded-lg border border-border/70 bg-muted/30 p-4 text-xs text-muted-foreground">
      <Trans i18nKey="settings:promptMentionHelp" values={{ token: PROMPT_MENTION_TOKEN }}>
        Use <span className="font-medium text-foreground">{PROMPT_MENTION_TOKEN}</span> in the chat
        input to insert a prompt’s content. Prompts are matched by name and expanded in place.
      </Trans>
    </div>
  );
}

export function PromptsSettings() {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const { isFinePointer } = useResponsiveBreakpoint();
  const state = usePromptsState();
  const actions = usePromptsActions(state);
  const { showCreate, createForm, setCreateForm, prompts, promptsLoaded } = state;
  const { isValid, isBusy } = actions;
  const draft = getPromptDraftMeta(state);
  const keepIds = activeRowIds(state);

  return (
    <SettingsPageTemplate
      title={t("common:prompts")}
      description={t("settings:promptsPageDescription")}
      isDirty={draft.isDirty}
      saveStatus="idle"
      saveId="prompts-item-draft"
      saveRevision={draft.revision}
      canSave={!draft.isDirty || isValid}
      invalidReason={draft.isDirty && !isValid ? t("settings:promptsInvalidReason") : undefined}
      onSave={showCreate ? actions.handleCreate : actions.handleUpdate}
      onDiscard={actions.discard}
    >
      <div className="space-y-6">
        <PromptConflictNotice />
        <PromptOrphanedDraftsNotice drafts={orphanedDrafts(prompts, state.drafts)} />
        <PromptMentionHelp />
        <PromptSyncSection />
        <SettingsGroup
          title={t("settings:promptsCustomHeading")}
          frame="none"
          contentClassName="space-y-6"
          data-testid="prompts-custom-section"
          action={
            <PromptCreateButton
              onClick={actions.startCreate}
              disabled={isBusy || showCreate || draft.isDirty}
            />
          }
        >
          {showCreate && (
            <PromptCreateForm
              formState={createForm}
              onFormChange={(patch) => setCreateForm((prev) => ({ ...prev, ...patch }))}
              onCancel={actions.closeCreate}
              isBusy={isBusy}
            />
          )}

          <div className="space-y-3">
            <PromptListSearch
              query={query}
              onQueryChange={setQuery}
              disabled={showCreate || isBusy || keepIds.size > 0}
            />
            <PromptListContent
              promptsLoaded={promptsLoaded || prompts.length > 0}
              hasPrompts={prompts.length > 0}
              prompts={filterSavedPrompts(prompts, query, keepIds)}
              state={state}
              actions={actions}
              isFinePointer={isFinePointer}
            />
          </div>
        </SettingsGroup>
      </div>
    </SettingsPageTemplate>
  );
}
