"use client";

import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { PromptDeleteConfirmation } from "@/components/settings/prompt-delete-confirmation";
import { PromptRowActions } from "@/components/settings/prompt-row-actions";
import {
  PromptOverrideSwitch,
  PromptRowBadges,
} from "@/components/settings/prompt-override-controls";
import { PromptSyncedBadge } from "@/components/settings/prompt-synced-badge";
import { SettingsPromptEditor } from "@/components/settings/settings-prompt-editor";
import {
  formFromPrompt,
  isLinkedPrompt,
  repositoryVersion,
  type PromptFormState,
} from "@/lib/settings/prompt-versions";
import type { CustomPrompt } from "@/lib/types/http";

export function getPromptPreview(content: string) {
  return content.split(/\r?\n/)[0] ?? "";
}

export type PromptListItemProps = {
  prompt: CustomPrompt;
  isEditing: boolean;
  editingRef: React.RefObject<HTMLDivElement | null>;
  formState: PromptFormState;
  onFormChange: (patch: Partial<PromptFormState>) => void;
  onStartEditing: (prompt: CustomPrompt, useLocal?: boolean) => void;
  onOpenDelete: (prompt: CustomPrompt) => void;
  onDeleteCancel: () => void;
  onDeleteConfirm: () => void;
  onCancel: () => void;
  isBusy: boolean;
  showCreate: boolean;
  isFinePointer: boolean;
  isDeleteTarget: boolean;
};

type PromptEditFormProps = Pick<
  PromptListItemProps,
  "prompt" | "formState" | "onFormChange" | "onCancel" | "isBusy"
>;

// RepositoryVersionView shows the repository version read-only while the
// Local override switch is off, with the file where it is edited.
function RepositoryVersionView({ prompt }: { prompt: CustomPrompt }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <SettingsPromptEditor
        value={repositoryVersion(prompt)}
        onChange={() => undefined}
        readOnly
        testId="prompt-content-input"
      />
      <p className="text-xs text-muted-foreground" data-testid="prompt-view-source-note">
        {t("settings:promptSyncedReadOnlyNote", { path: prompt.source_path ?? "" })}
      </p>
    </div>
  );
}

// PromptEditForm edits a prompt. For a synced prompt or an override the name
// belongs to its repository file, and the editor shows the version the switch
// selects: the local version editable, or the repository version read-only.
function PromptEditForm({
  prompt,
  formState,
  onFormChange,
  onCancel,
  isBusy,
}: PromptEditFormProps) {
  const { t } = useTranslation();
  const saved = formFromPrompt(prompt);
  const linked = isLinkedPrompt(prompt);
  return (
    <div className="space-y-3">
      <Input
        value={formState.name}
        onChange={(event) => onFormChange({ name: event.target.value })}
        placeholder={t("settings:promptNamePlaceholder")}
        data-testid="prompt-name-input"
        disabled={isBusy || linked}
        data-settings-dirty={formState.name !== saved.name}
      />
      {linked && !formState.useLocal ? (
        <RepositoryVersionView prompt={prompt} />
      ) : (
        <SettingsPromptEditor
          value={formState.content}
          onChange={(value) => onFormChange({ content: value })}
          promptReferences
          excludedPromptIds={[prompt.id]}
          readOnly={isBusy}
          testId="prompt-content-input"
          isDirty={formState.content !== saved.content}
          dirtyLevel="field"
        />
      )}
      <div className="flex items-center gap-2">
        <Button variant="ghost" onClick={onCancel} disabled={isBusy}>
          {t("settings:cancel")}
        </Button>
      </div>
    </div>
  );
}

// PromptListItem renders an editable prompt row, and any prompt open for
// editing. Toggling the Local override switch is a pending edit: on a row not
// open for editing it opens the row with the switch changed.
export function PromptListItem({
  prompt,
  isEditing,
  editingRef,
  formState,
  onFormChange,
  onStartEditing,
  onOpenDelete,
  onDeleteCancel,
  onDeleteConfirm,
  onCancel,
  isBusy,
  showCreate,
  isFinePointer,
  isDeleteTarget,
}: PromptListItemProps) {
  const deleteAnchorRef = useRef<HTMLButtonElement | null>(null);
  const synced = prompt.source === "sync";
  const draftDirty =
    isEditing && JSON.stringify(formState) !== JSON.stringify(formFromPrompt(prompt));
  return (
    <div
      className="rounded-lg border border-border/70 bg-background p-4 flex flex-col gap-3"
      ref={isEditing ? editingRef : null}
      data-testid="prompt-list-item"
      data-prompt-name={prompt.name}
      data-settings-dirty={draftDirty}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <div className="text-sm font-medium text-foreground">@{prompt.name}</div>
          {synced && <PromptSyncedBadge sourcePath={prompt.source_path} />}
          <PromptRowBadges prompt={prompt} />
        </div>
        <div className="flex shrink-0 items-center gap-1" data-testid="prompt-row-trailing">
          {isLinkedPrompt(prompt) && (
            <PromptOverrideSwitch
              prompt={prompt}
              checked={isEditing ? formState.useLocal === true : prompt.source === "override"}
              disabled={isBusy || showCreate}
              onCheckedChange={(checked) =>
                isEditing ? onFormChange({ useLocal: checked }) : onStartEditing(prompt, checked)
              }
            />
          )}
          {!synced && (
            <PromptRowActions
              prompt={prompt}
              deleteAnchorRef={deleteAnchorRef}
              onStartEditing={onStartEditing}
              onOpenDelete={onOpenDelete}
              isBusy={isBusy}
              showCreate={showCreate}
              isFinePointer={isFinePointer}
              isDeleteTarget={isDeleteTarget}
            />
          )}
        </div>
      </div>
      <PromptDeleteConfirmation
        promptId={prompt.id}
        promptName={prompt.name}
        open={isDeleteTarget}
        isFinePointer={isFinePointer}
        anchorRef={deleteAnchorRef}
        isBusy={isBusy}
        onClose={onDeleteCancel}
        onCancel={onDeleteCancel}
        onConfirm={onDeleteConfirm}
      />
      {isEditing ? (
        <PromptEditForm
          prompt={prompt}
          formState={formState}
          onFormChange={onFormChange}
          onCancel={onCancel}
          isBusy={isBusy}
        />
      ) : (
        <div className="text-xs text-muted-foreground whitespace-pre-wrap">
          <div className="truncate">{getPromptPreview(prompt.content)}</div>
        </div>
      )}
    </div>
  );
}
