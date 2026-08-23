"use client";

import { useCallback, useMemo, useRef, useState, useEffect } from "react";
import { Trans, useTranslation } from "react-i18next";
import { IconLock } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Input } from "@kandev/ui/input";
import { PromptDeleteConfirmation } from "@/components/settings/prompt-delete-confirmation";
import { PromptRowActions } from "@/components/settings/prompt-row-actions";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { SettingsPromptEditor } from "@/components/settings/settings-prompt-editor";
import { useToast } from "@/components/toast-provider";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import { createPrompt, deletePrompt, updatePrompt } from "@/lib/api";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { useRequest } from "@/lib/http/use-request";
import { t } from "@/lib/i18n";
import type { CustomPrompt } from "@/lib/types/http";

const defaultFormState = {
  name: "",
  content: "",
};

/** The sigil the chat input matches on. Typed verbatim, so never translated. */
const PROMPT_MENTION_TOKEN = "@name";

function errorMessage(error: unknown): string {
  // A thrown Error carries the backend's diagnostic, which stays English by
  // design; only the missing-payload fallback is copy.
  return error instanceof Error ? error.message : t("common:requestFailed");
}

function getPromptPreview(content: string) {
  return content.split(/\r?\n/)[0] ?? "";
}

async function runPromptSave(
  action: () => Promise<unknown>,
  reportError: (error: unknown) => void,
) {
  try {
    await action();
  } catch (error) {
    reportError(error);
    throw error;
  }
}

type PromptFormState = typeof defaultFormState;

type PromptCreateFormProps = {
  formState: PromptFormState;
  onFormChange: (patch: Partial<PromptFormState>) => void;
  onCancel: () => void;
  isBusy: boolean;
};

function PromptCreateForm({ formState, onFormChange, onCancel, isBusy }: PromptCreateFormProps) {
  const { t } = useTranslation();
  const nameIsDirty = formState.name !== defaultFormState.name;
  const contentIsDirty = formState.content !== defaultFormState.content;
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

type PromptListItemProps = {
  prompt: CustomPrompt;
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

function PromptListItem({
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
  const { t } = useTranslation();
  const deleteAnchorRef = useRef<HTMLButtonElement | null>(null);
  const nameIsDirty = isEditing && formState.name !== prompt.name;
  const contentIsDirty = isEditing && formState.content !== prompt.content;

  return (
    <div
      className="rounded-lg border border-border/70 bg-background p-4 flex flex-col gap-3"
      ref={isEditing ? editingRef : null}
      data-testid="prompt-list-item"
      data-prompt-name={prompt.name}
      data-settings-dirty={nameIsDirty || contentIsDirty}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <div className="text-sm font-medium text-foreground">@{prompt.name}</div>
          {prompt.builtin && (
            <Badge variant="secondary" className="text-xs">
              <IconLock className="h-3 w-3 mr-1" />
              {t("settings:builtIn")}
            </Badge>
          )}
        </div>
        <PromptRowActions
          prompt={prompt}
          deleteAnchorRef={deleteAnchorRef}
          onStartEditing={onStartEditing}
          onOpenDelete={onOpenDelete}
          onDeleteClose={onDeleteCancel}
          onDeleteCancel={onDeleteCancel}
          onDeleteConfirm={onDeleteConfirm}
          isBusy={isBusy}
          showCreate={showCreate}
          isFinePointer={isFinePointer}
          isDeleteTarget={isDeleteTarget}
        />
      </div>
      {!isFinePointer ? (
        <PromptDeleteConfirmation
          promptName={prompt.name}
          open={isDeleteTarget}
          isFinePointer={isFinePointer}
          anchorRef={deleteAnchorRef}
          isBusy={isBusy}
          onClose={onDeleteCancel}
          onCancel={onDeleteCancel}
          onConfirm={onDeleteConfirm}
        />
      ) : null}
      {isEditing ? (
        <div className="space-y-3">
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
            excludedPromptIds={[prompt.id]}
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
      ) : (
        <div className="text-xs text-muted-foreground whitespace-pre-wrap">
          <div className="truncate">{getPromptPreview(prompt.content)}</div>
        </div>
      )}
    </div>
  );
}

type PromptListContentProps = {
  promptsLoaded: boolean;
  prompts: CustomPrompt[];
  editingId: string | null;
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
  deleteTargetId: string | null;
};

function PromptListContent({
  promptsLoaded,
  prompts,
  editingId,
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
  deleteTargetId,
}: PromptListContentProps) {
  const { t } = useTranslation();
  if (!promptsLoaded) {
    return (
      <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
        {t("settings:promptsLoading")}
      </div>
    );
  }
  if (prompts.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
        {t("settings:promptsEmpty")}
      </div>
    );
  }
  return (
    <>
      {prompts.map((prompt: CustomPrompt) => (
        <PromptListItem
          key={prompt.id}
          prompt={prompt}
          isEditing={editingId === prompt.id}
          editingRef={editingRef}
          formState={formState}
          onFormChange={onFormChange}
          onStartEditing={onStartEditing}
          onOpenDelete={onOpenDelete}
          onDeleteCancel={onDeleteCancel}
          onDeleteConfirm={onDeleteConfirm}
          onCancel={onCancel}
          isBusy={isBusy}
          showCreate={showCreate}
          isFinePointer={isFinePointer}
          isDeleteTarget={deleteTargetId === prompt.id}
        />
      ))}
    </>
  );
}

function usePromptsState() {
  const { loaded: promptsLoaded } = useCustomPrompts();
  const prompts = useAppStore((state) => state.prompts.items);
  const setPrompts = useAppStore((state) => state.setPrompts);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [formState, setFormState] = useState(defaultFormState);
  const editingRef = useRef<HTMLDivElement | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<CustomPrompt | null>(null);
  return {
    promptsLoaded,
    prompts,
    setPrompts,
    editingId,
    setEditingId,
    showCreate,
    setShowCreate,
    formState,
    setFormState,
    editingRef,
    deleteTarget,
    setDeleteTarget,
  };
}

/** The three prompt mutations, sharing the post-write list refresh and reset. */
function usePromptRequests(
  prompts: CustomPrompt[],
  setPrompts: (next: CustomPrompt[]) => void,
  editingId: string | null,
  resetForm: () => void,
) {
  const applyPrompts = useCallback(
    (next: CustomPrompt[]) => {
      setPrompts([...next].sort((a, b) => a.name.localeCompare(b.name)));
    },
    [setPrompts],
  );

  const createRequest = useRequest(async (s: typeof defaultFormState) => {
    const prompt = await createPrompt(
      { name: s.name.trim(), content: s.content.trim() },
      { cache: "no-store" },
    );
    applyPrompts([...prompts, prompt]);
    resetForm();
  });

  const updateRequest = useRequest(async (id: string, s: typeof defaultFormState) => {
    const updated = await updatePrompt(
      id,
      { name: s.name.trim(), content: s.content.trim() },
      { cache: "no-store" },
    );
    applyPrompts(prompts.map((p: CustomPrompt) => (p.id === id ? updated : p)));
    resetForm();
  });

  const deleteRequest = useRequest(async (id: string) => {
    await deletePrompt(id, { cache: "no-store" });
    applyPrompts(prompts.filter((p: CustomPrompt) => p.id !== id));
    if (editingId === id) resetForm();
  });

  return { createRequest, updateRequest, deleteRequest };
}

function usePromptsActions(state: ReturnType<typeof usePromptsState>) {
  const {
    prompts,
    setPrompts,
    editingId,
    setEditingId,
    setShowCreate,
    setFormState,
    setDeleteTarget,
    deleteTarget,
    formState,
  } = state;
  const { toast } = useToast();

  const resetForm = useCallback(() => {
    setEditingId(null);
    setShowCreate(false);
    setFormState(defaultFormState);
  }, [setEditingId, setShowCreate, setFormState]);

  const isValid = useMemo(
    () => Boolean(formState.name.trim() && formState.content.trim()),
    [formState],
  );

  const { createRequest, updateRequest, deleteRequest } = usePromptRequests(
    prompts,
    setPrompts,
    editingId,
    resetForm,
  );

  const isBusy = createRequest.isLoading || updateRequest.isLoading || deleteRequest.isLoading;
  const toastError = (title: string) => (err: unknown) =>
    toast({ title, description: errorMessage(err), variant: "error" });
  const handleCreate = () => {
    if (!isValid || isBusy) return;
    return runPromptSave(
      () => createRequest.run(formState),
      toastError(t("settings:promptCreateFailed")),
    );
  };
  const handleUpdate = () => {
    if (!isValid || isBusy || !editingId) return;
    return runPromptSave(
      () => updateRequest.run(editingId, formState),
      toastError(t("settings:promptSaveFailed")),
    );
  };
  const startEditing = (prompt: CustomPrompt) => {
    setEditingId(prompt.id);
    setShowCreate(false);
    setFormState({ name: prompt.name, content: prompt.content });
  };
  const startCreate = () => {
    setEditingId(null);
    setShowCreate(true);
    setFormState(defaultFormState);
  };
  const openDeleteDialog = (prompt: CustomPrompt) => {
    setDeleteTarget(prompt);
  };
  const closeDeleteDialog = () => {
    setDeleteTarget(null);
  };
  const confirmDelete = () => {
    if (!deleteTarget || isBusy) return;
    return deleteRequest.run(deleteTarget.id).catch(toastError(t("settings:promptDeleteFailed")));
  };

  return {
    resetForm,
    isValid,
    isBusy,
    handleCreate,
    handleUpdate,
    startEditing,
    startCreate,
    openDeleteDialog,
    closeDeleteDialog,
    confirmDelete,
  };
}

export function getPromptDraftMeta(
  prompts: CustomPrompt[],
  editingId: string | null,
  showCreate: boolean,
  formState: PromptFormState,
) {
  const editingPrompt = prompts.find((prompt) => prompt.id === editingId);
  const revision = JSON.stringify(formState);
  const savedRevision = JSON.stringify({
    name: editingPrompt?.name ?? "",
    content: editingPrompt?.content ?? "",
  });
  return {
    isDirty: showCreate || (Boolean(editingId) && revision !== savedRevision),
    revision: `${showCreate ? "new" : (editingId ?? "none")}:${revision}`,
  };
}

export function PromptsSettings() {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const state = usePromptsState();
  const {
    editingId,
    showCreate,
    formState,
    setFormState,
    editingRef,
    deleteTarget,
    promptsLoaded,
    prompts,
  } = state;
  const {
    isValid,
    isBusy,
    handleCreate,
    handleUpdate,
    startEditing,
    startCreate,
    openDeleteDialog,
    closeDeleteDialog,
    confirmDelete,
    resetForm,
  } = usePromptsActions(state);
  const isEditing = Boolean(editingId);
  const draft = getPromptDraftMeta(prompts, editingId, showCreate, formState);

  useEffect(() => {
    if (!editingId) return;
    editingRef.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [editingId, editingRef]);

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
      onSave={showCreate ? handleCreate : handleUpdate}
      onDiscard={resetForm}
    >
      <div className="rounded-lg border border-border/70 bg-muted/30 p-4 text-xs text-muted-foreground">
        <Trans i18nKey="settings:promptMentionHelp" values={{ token: PROMPT_MENTION_TOKEN }}>
          Use <span className="font-medium text-foreground">{PROMPT_MENTION_TOKEN}</span> in the
          chat input to insert a prompt’s content. Prompts are matched by name and expanded in
          place.
        </Trans>
      </div>
      <div className="space-y-6 mt-4">
        <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
          <div className="text-sm font-medium text-foreground">
            {t("settings:promptsCustomHeading")}
          </div>
          <Button
            onClick={startCreate}
            disabled={isBusy || isEditing || showCreate}
            className={settingsActionClassName()}
            data-testid="prompt-create-button"
          >
            {t("settings:promptAdd")}
          </Button>
        </div>

        {showCreate && (
          <PromptCreateForm
            formState={formState}
            onFormChange={(patch) => setFormState((prev) => ({ ...prev, ...patch }))}
            onCancel={resetForm}
            isBusy={isBusy}
          />
        )}

        <div className="space-y-3">
          <PromptListContent
            promptsLoaded={promptsLoaded}
            prompts={prompts}
            editingId={editingId}
            editingRef={editingRef}
            formState={formState}
            onFormChange={(patch) => setFormState((prev) => ({ ...prev, ...patch }))}
            onStartEditing={startEditing}
            onOpenDelete={openDeleteDialog}
            onDeleteCancel={closeDeleteDialog}
            onDeleteConfirm={confirmDelete}
            onCancel={resetForm}
            isBusy={isBusy}
            showCreate={showCreate}
            isFinePointer={isFinePointer}
            deleteTargetId={deleteTarget?.id ?? null}
          />
        </div>
      </div>
    </SettingsPageTemplate>
  );
}
