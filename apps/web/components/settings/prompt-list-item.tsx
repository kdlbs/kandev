"use client";

import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { PromptDeleteConfirmation } from "@/components/settings/prompt-delete-confirmation";
import { PromptRowActions } from "@/components/settings/prompt-row-actions";
import {
  PromptRowBadges,
  PromptVersionControl,
} from "@/components/settings/prompt-override-controls";
import { PromptSyncedBadge } from "@/components/settings/prompt-synced-badge";
import { PromptVersionPreview } from "@/components/settings/prompt-version-preview";
import { SettingsPromptEditor } from "@/components/settings/settings-prompt-editor";
import { formFromPrompt, type PromptFormState } from "@/lib/settings/prompt-versions";
import type { CustomPrompt } from "@/lib/types/http";
import type { PromptConflictChoice } from "@/lib/types/prompt-sync";

export function getPromptPreview(content: string) {
  return content.split(/\r?\n/)[0] ?? "";
}

/** The version control state of a synced prompt or an override. */
export type PromptRowVersion = {
  choice: PromptConflictChoice;
  /** The choice differs from the saved one and waits for Save. */
  pending: boolean;
  onChoose: (choice: PromptConflictChoice) => void;
};

export type PromptListItemProps = {
  prompt: CustomPrompt;
  /** Present for synced prompts and overrides. */
  version?: PromptRowVersion;
  isEditing: boolean;
  editingRef: React.RefObject<HTMLDivElement | null>;
  formState: PromptFormState;
  onFormChange: (patch: Partial<PromptFormState>) => void;
  onStartEditing: (prompt: CustomPrompt) => void;
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
  "prompt" | "version" | "formState" | "onFormChange" | "onCancel" | "isBusy"
>;

// PromptEditForm edits a prompt. For a synced prompt or an override the name
// belongs to its repository file, and the editor shows the version the
// control selects: the local version editable, or the repository version
// read-only.
function PromptEditForm({
  prompt,
  version,
  formState,
  onFormChange,
  onCancel,
  isBusy,
}: PromptEditFormProps) {
  const { t } = useTranslation();
  const saved = formFromPrompt(prompt);
  return (
    <div className="space-y-3">
      <Input
        value={formState.name}
        onChange={(event) => onFormChange({ name: event.target.value })}
        placeholder={t("settings:promptNamePlaceholder")}
        data-testid="prompt-name-input"
        disabled={isBusy || Boolean(version)}
        data-settings-dirty={formState.name !== saved.name}
      />
      {version?.choice === "repository" ? (
        <PromptVersionPreview prompt={prompt} choice="repository" testId="prompt-content-input" />
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

function PromptRowBody(props: PromptListItemProps) {
  const { prompt, version, isEditing } = props;
  if (isEditing) return <PromptEditForm {...props} />;
  if (version?.pending) return <PromptVersionPreview prompt={prompt} choice={version.choice} />;
  return (
    <div className="text-xs text-muted-foreground whitespace-pre-wrap">
      <div className="truncate">{getPromptPreview(prompt.content)}</div>
    </div>
  );
}

// PromptListItem renders an editable prompt row, an override, and any prompt
// open for editing. A switched row previews the version it will use.
export function PromptListItem(props: PromptListItemProps) {
  const {
    prompt,
    version,
    isEditing,
    editingRef,
    formState,
    onStartEditing,
    onOpenDelete,
    onDeleteCancel,
    onDeleteConfirm,
    isBusy,
    showCreate,
    isFinePointer,
    isDeleteTarget,
  } = props;
  const deleteAnchorRef = useRef<HTMLButtonElement | null>(null);
  const synced = prompt.source === "sync";
  const saved = formFromPrompt(prompt);
  const draftDirty =
    isEditing && (formState.name !== saved.name || formState.content !== saved.content);
  return (
    <div
      className="rounded-lg border border-border/70 bg-background p-4 flex flex-col gap-3"
      ref={isEditing ? editingRef : null}
      data-testid="prompt-list-item"
      data-prompt-name={prompt.name}
      data-settings-dirty={draftDirty || Boolean(version?.pending)}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <div className="text-sm font-medium text-foreground">@{prompt.name}</div>
          {synced && <PromptSyncedBadge sourcePath={prompt.source_path} />}
          <PromptRowBadges prompt={prompt} />
        </div>
        <div className="flex shrink-0 items-center gap-1" data-testid="prompt-row-trailing">
          {version && (
            <PromptVersionControl
              prompt={prompt}
              choice={version.choice}
              disabled={isBusy || showCreate}
              onChoose={version.onChoose}
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
      <PromptRowBody {...props} />
    </div>
  );
}
