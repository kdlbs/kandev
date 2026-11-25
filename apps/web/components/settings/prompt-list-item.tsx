"use client";

import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { PromptDeleteConfirmation } from "@/components/settings/prompt-delete-confirmation";
import { PromptRepositoryView } from "@/components/settings/prompt-repository-view";
import { PromptRowActions } from "@/components/settings/prompt-row-actions";
import {
  PromptRowBadges,
  PromptVersionControl,
} from "@/components/settings/prompt-override-controls";
import { PromptSyncedBadge } from "@/components/settings/prompt-synced-badge";
import { SettingsPromptEditor } from "@/components/settings/settings-prompt-editor";
import {
  formFromPrompt,
  repositoryVersion,
  type PromptFormState,
} from "@/lib/settings/prompt-versions";
import type { CustomPrompt } from "@/lib/types/http";
import type { PromptConflictChoice } from "@/lib/types/prompt-sync";

function getPromptPreview(content: string) {
  return content.split(/\r?\n/)[0] ?? "";
}

/** The version control state of a synced prompt or an override. */
type PromptRowVersion = {
  choice: PromptConflictChoice;
  /** The choice differs from the saved one and waits for Save. */
  pending: boolean;
  onChoose: (choice: PromptConflictChoice) => void;
};

type PromptListItemProps = {
  prompt: CustomPrompt;
  /** Present for synced prompts and overrides. */
  version?: PromptRowVersion;
  /** The row shows its text: editable, or read-only on Repository. */
  open: boolean;
  onToggleOpen: (prompt: CustomPrompt) => void;
  /** The row's unsaved edit, or the saved prompt. */
  form: PromptFormState;
  /** The row has an unsaved edit. */
  edited: boolean;
  onFormChange: (prompt: CustomPrompt, patch: Partial<PromptFormState>) => void;
  /** Discards the row's unsaved edit and closes it. */
  onCancel: (prompt: CustomPrompt) => void;
  onOpenDelete: (prompt: CustomPrompt) => void;
  onDeleteCancel: () => void;
  onDeleteConfirm: () => void;
  isBusy: boolean;
  showCreate: boolean;
  isFinePointer: boolean;
  isDeleteTarget: boolean;
};

type PromptEditFormProps = Pick<
  PromptListItemProps,
  "prompt" | "version" | "form" | "onFormChange" | "onCancel" | "isBusy"
>;

// PromptEditForm edits a prompt's text. A synced prompt's or an override's
// name belongs to its repository file, so only a manual prompt edits it.
function PromptEditForm({
  prompt,
  version,
  form,
  onFormChange,
  onCancel,
  isBusy,
}: PromptEditFormProps) {
  const { t } = useTranslation();
  const saved = formFromPrompt(prompt);
  return (
    <div className="space-y-3">
      {!version && (
        <Input
          value={form.name}
          onChange={(event) => onFormChange(prompt, { name: event.target.value })}
          placeholder={t("settings:promptNamePlaceholder")}
          data-testid="prompt-name-input"
          disabled={isBusy}
          data-settings-dirty={form.name !== saved.name}
        />
      )}
      <SettingsPromptEditor
        value={form.content}
        onChange={(value) => onFormChange(prompt, { content: value })}
        promptReferences
        excludedPromptIds={[prompt.id]}
        readOnly={isBusy}
        testId="prompt-content-input"
        isDirty={form.content !== saved.content}
        dirtyLevel="field"
      />
      <div className="flex items-center gap-2">
        <Button variant="ghost" onClick={() => onCancel(prompt)} disabled={isBusy}>
          {t("settings:cancel")}
        </Button>
      </div>
    </div>
  );
}

function PromptRowBody(props: PromptListItemProps & { editable: boolean }) {
  const { prompt, open, editable, form } = props;
  if (open && editable) return <PromptEditForm {...props} />;
  if (open) return <PromptRepositoryView prompt={prompt} />;
  return (
    <div className="text-xs text-muted-foreground whitespace-pre-wrap">
      <div className="truncate">
        {getPromptPreview(editable ? form.content : repositoryVersion(prompt))}
      </div>
    </div>
  );
}

function PromptUnsavedBadge() {
  const { t } = useTranslation();
  return (
    <Badge
      variant="outline"
      className="text-xs border-success/60 text-success"
      data-testid="prompt-unsaved-badge"
    >
      {t("settings:promptUnsaved")}
    </Badge>
  );
}

// PromptListItem renders one saved prompt. Edit shows or hides its editable
// text; a synced prompt or an override on Repository shows its repository
// version read-only from the eye instead. Switching versions never opens or
// closes the row.
export function PromptListItem(props: PromptListItemProps) {
  const { prompt, version, open, edited, onToggleOpen, onOpenDelete, isDeleteTarget } = props;
  const deleteAnchorRef = useRef<HTMLButtonElement | null>(null);
  const synced = prompt.source === "sync";
  const editable = !version || version.choice === "local";
  const unsaved = edited || Boolean(version?.pending);
  return (
    <div
      className="rounded-lg border border-border/70 bg-background p-4 flex flex-col gap-3"
      data-testid="prompt-list-item"
      data-prompt-name={prompt.name}
      data-settings-dirty={unsaved}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <div className="text-sm font-medium text-foreground">@{prompt.name}</div>
          {synced && <PromptSyncedBadge sourcePath={prompt.source_path} />}
          <PromptRowBadges prompt={prompt} />
          {unsaved && <PromptUnsavedBadge />}
        </div>
        <div className="flex shrink-0 items-center gap-1" data-testid="prompt-row-trailing">
          {version && (
            <PromptVersionControl
              prompt={prompt}
              choice={version.choice}
              disabled={props.isBusy || props.showCreate}
              onChoose={version.onChoose}
            />
          )}
          <PromptRowActions
            prompt={prompt}
            deleteAnchorRef={deleteAnchorRef}
            editable={editable}
            open={open}
            onToggleOpen={onToggleOpen}
            canDelete={!synced}
            onOpenDelete={onOpenDelete}
            isBusy={props.isBusy}
            showCreate={props.showCreate}
            isFinePointer={props.isFinePointer}
            isDeleteTarget={isDeleteTarget}
          />
        </div>
      </div>
      <PromptDeleteConfirmation
        promptId={prompt.id}
        promptName={prompt.name}
        open={isDeleteTarget}
        isFinePointer={props.isFinePointer}
        anchorRef={deleteAnchorRef}
        isBusy={props.isBusy}
        onClose={props.onDeleteCancel}
        onCancel={props.onDeleteCancel}
        onConfirm={props.onDeleteConfirm}
      />
      <div id={`prompt-body-${prompt.id}`}>
        <PromptRowBody {...props} editable={editable} />
      </div>
    </div>
  );
}
