"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { PromptListItem } from "@/components/settings/prompt-list-item";
import { SyncedPromptListItem } from "@/components/settings/prompt-synced-view";
import { PromptSyncSection } from "@/components/settings/prompt-sync-section";
import { PromptConflictNotice } from "@/components/settings/prompt-conflict-review-dialog";
import {
  filterSavedPrompts,
  PromptCreateButton,
  PromptListSearch,
} from "@/components/settings/prompt-list-search";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { SettingsGroup } from "@/components/settings/settings-group";
import { SettingsPromptEditor } from "@/components/settings/settings-prompt-editor";
import { useToast } from "@/components/toast-provider";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { createPrompt, deletePrompt, updatePrompt } from "@/lib/api";
import { savePromptVersions } from "@/lib/api/domains/prompt-sync-api";
import {
  choiceFor,
  formFromPrompt,
  isLinkedPrompt,
  planPromptSaves,
  withChoice,
  type PendingVersions,
  type PromptFormState,
  type PromptSave,
} from "@/lib/settings/prompt-versions";
import type { PromptConflictChoice } from "@/lib/types/prompt-sync";
import { useRequest } from "@/lib/http/use-request";
import { t } from "@/lib/i18n";
import type { CustomPrompt } from "@/lib/types/http";
import { type PromptEditBaseline, usePromptEditorEffects } from "./use-prompt-editor-effects";

const defaultFormState: PromptFormState = {
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

type PromptListContentProps = {
  promptsLoaded: boolean;
  prompts: CustomPrompt[];
  hasPrompts: boolean;
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
  pending: PendingVersions;
  onChoose: (prompt: CustomPrompt, choice: PromptConflictChoice) => void;
};

function PromptListContent({
  promptsLoaded,
  prompts,
  hasPrompts,
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
  pending,
  onChoose,
}: PromptListContentProps) {
  const { t } = useTranslation();
  if (!promptsLoaded) {
    return (
      <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
        {t("settings:promptsLoading")}
      </div>
    );
  }
  if (!hasPrompts) {
    return (
      <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
        {t("settings:promptsEmpty")}
      </div>
    );
  }
  if (prompts.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
        {t("common:noResults")}
      </div>
    );
  }
  return (
    <>
      {prompts.map((prompt: CustomPrompt) => {
        const version = isLinkedPrompt(prompt)
          ? {
              choice: choiceFor(prompt, pending),
              pending: pending[prompt.id] !== undefined,
              onChoose: (choice: PromptConflictChoice) => onChoose(prompt, choice),
            }
          : undefined;
        return prompt.source === "sync" && version && editingId !== prompt.id ? (
          <SyncedPromptListItem
            key={prompt.id}
            prompt={prompt}
            version={version}
            onEdit={onStartEditing}
            disabled={isBusy || showCreate}
          />
        ) : (
          <PromptListItem
            key={prompt.id}
            prompt={prompt}
            version={version}
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
        );
      })}
    </>
  );
}

function usePromptsState() {
  const { loaded: promptsLoaded } = useCustomPrompts();
  const prompts = useAppStore((state) => state.prompts.items);
  const setPrompts = useAppStore((state) => state.setPrompts);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editBaseline, setEditBaseline] = useState<PromptEditBaseline | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [formState, setFormState] = useState(defaultFormState);
  const editingRef = useRef<HTMLDivElement | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<CustomPrompt | null>(null);
  const [pending, setPending] = useState<PendingVersions>({});
  return {
    pending,
    setPending,
    promptsLoaded,
    prompts,
    setPrompts,
    editingId,
    setEditingId,
    editBaseline,
    setEditBaseline,
    showCreate,
    setShowCreate,
    formState,
    setFormState,
    editingRef,
    deleteTarget,
    setDeleteTarget,
  };
}

/** The prompt mutations, sharing the post-write list refresh and reset. */
function usePromptRequests(
  setPrompts: (next: CustomPrompt[]) => void,
  editingId: string | null,
  resetForm: () => void,
  setPending: React.Dispatch<React.SetStateAction<PendingVersions>>,
) {
  const store = useAppStoreApi();
  const applyPrompts = useCallback(
    (change: (current: CustomPrompt[]) => CustomPrompt[]) => {
      setPrompts(
        change(store.getState().prompts.items).sort((a, b) => a.name.localeCompare(b.name)),
      );
    },
    [setPrompts, store],
  );

  const createRequest = useRequest(async (s: typeof defaultFormState) => {
    const prompt = await createPrompt(
      { name: s.name.trim(), content: s.content.trim() },
      { cache: "no-store" },
    );
    applyPrompts((current) => [...current.filter((p) => p.id !== prompt.id), prompt]);
    resetForm();
  });

  // saveRequest makes one Save's writes in order. Each written prompt leaves
  // the pending switches at once, so a failure keeps only what is unsaved.
  const saveRequest = useRequest(async (plan: PromptSave[]) => {
    for (const save of plan) {
      const updated =
        save.kind === "versions"
          ? await savePromptVersions(save.prompt.id, save.payload, { cache: "no-store" })
          : await updatePrompt(
              save.prompt.id,
              {
                name: save.form.name.trim(),
                content: save.form.content.trim(),
                expected_updated_at: save.expectedUpdatedAt,
              },
              { cache: "no-store" },
            );
      applyPrompts((current) => current.map((p) => (p.id === updated.id ? updated : p)));
      setPending((current) => {
        const { [updated.id]: _saved, ...rest } = current;
        return rest;
      });
    }
    resetForm();
  });

  const deleteRequest = useRequest(async (id: string) => {
    await deletePrompt(id, { cache: "no-store" });
    applyPrompts((current) => current.filter((p) => p.id !== id));
    if (editingId === id) resetForm();
  });

  return { createRequest, saveRequest, deleteRequest };
}

function usePromptsActions(state: ReturnType<typeof usePromptsState>) {
  const {
    setPrompts,
    editingId,
    setEditingId,
    editBaseline,
    setEditBaseline,
    setShowCreate,
    setFormState,
    setDeleteTarget,
    deleteTarget,
    formState,
    showCreate,
    pending,
    setPending,
  } = state;
  const { toast } = useToast();

  const resetForm = useCallback(() => {
    setEditingId(null);
    setShowCreate(false);
    setFormState(defaultFormState);
    setEditBaseline(null);
  }, [setEditingId, setShowCreate, setFormState, setEditBaseline]);

  // Only an open draft has fields to validate; pending switches alone are valid.
  const isValid = useMemo(
    () => (!editingId && !showCreate) || Boolean(formState.name.trim() && formState.content.trim()),
    [editingId, showCreate, formState],
  );

  const { createRequest, saveRequest, deleteRequest } = usePromptRequests(
    setPrompts,
    editingId,
    resetForm,
    setPending,
  );

  const isBusy = [createRequest, saveRequest, deleteRequest].some((request) => request.isLoading);
  const toastError = (title: string) => (err: unknown) =>
    toast({ title, description: errorMessage(err), variant: "error" });
  const handleCreate = () => {
    if (!isValid || isBusy) return;
    return runPromptSave(
      () => createRequest.run(formState),
      toastError(t("settings:promptCreateFailed")),
    );
  };
  // handleUpdate saves every pending version switch and the open draft.
  const handleUpdate = () => {
    if (!isValid || isBusy) return;
    const editingPrompt = state.prompts.find((prompt) => prompt.id === editingId);
    const editing =
      editingPrompt && editBaseline
        ? { prompt: editingPrompt, form: formState, updatedAt: editBaseline.updated_at }
        : undefined;
    const plan = planPromptSaves({ prompts: state.prompts, pending, editing });
    return runPromptSave(() => saveRequest.run(plan), toastError(t("settings:promptSaveFailed")));
  };
  const startEditing = (prompt: CustomPrompt) => {
    const saved = formFromPrompt(prompt);
    setEditingId(prompt.id);
    setShowCreate(false);
    setEditBaseline({ ...saved, updated_at: prompt.updated_at });
    setFormState(saved);
  };
  const choose = (prompt: CustomPrompt, choice: PromptConflictChoice) =>
    setPending((current) => withChoice(current, prompt, choice));
  // discard resets the open draft and every pending switch.
  const discard = () => {
    resetForm();
    setPending({});
  };
  const startCreate = () => {
    setEditingId(null);
    setShowCreate(true);
    setEditBaseline(null);
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
    discard,
    choose,
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

function savedForm(
  editBaseline: PromptEditBaseline | null | undefined,
  editingPrompt: CustomPrompt | undefined,
): PromptFormState {
  if (editBaseline) {
    const { updated_at: _revision, ...form } = editBaseline;
    return form;
  }
  return editingPrompt ? formFromPrompt(editingPrompt) : defaultFormState;
}

export function getPromptDraftMeta(
  prompts: CustomPrompt[],
  editingId: string | null,
  showCreate: boolean,
  formState: PromptFormState,
  {
    editBaseline,
    pending = {},
  }: { editBaseline?: PromptEditBaseline | null; pending?: PendingVersions } = {},
) {
  const editingPrompt = prompts.find((prompt) => prompt.id === editingId);
  const revision = JSON.stringify(formState);
  const savedRevision = JSON.stringify(savedForm(editBaseline, editingPrompt));
  const switched = Object.keys(pending).length > 0;
  const editingDirty = Boolean(editingId) && revision !== savedRevision;
  return {
    isDirty: showCreate || editingDirty || switched,
    revision: `${showCreate ? "new" : (editingId ?? "none")}:${revision}${switched ? `:${JSON.stringify(pending)}` : ""}`,
  };
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

// usePromptsView derives the page's draft state and the visible prompt list,
// and keeps a pristine open draft in step with newer saved revisions.
function usePromptsView(state: ReturnType<typeof usePromptsState>, query: string, isBusy: boolean) {
  const { prompts, editingId, showCreate, formState, editBaseline, pending, deleteTarget } = state;
  const draft = getPromptDraftMeta(prompts, editingId, showCreate, formState, {
    editBaseline,
    pending,
  });
  const editingPrompt = prompts.find((prompt) => prompt.id === editingId);
  const deleteTargetId = deleteTarget?.id ?? null;
  usePromptEditorEffects({ ...state, editingPrompt });
  return {
    draft,
    deleteTargetId,
    visiblePrompts: filterSavedPrompts(prompts, query, editingId, deleteTargetId),
    searchDisabled: Boolean(editingId || showCreate || deleteTarget || isBusy),
  };
}

export function PromptsSettings() {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const { isFinePointer } = useResponsiveBreakpoint();
  const state = usePromptsState();
  const {
    editingId,
    showCreate,
    formState,
    setFormState,
    editingRef,
    promptsLoaded,
    prompts,
    pending,
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
    discard,
    choose,
  } = usePromptsActions(state);
  const { draft, deleteTargetId, visiblePrompts, searchDisabled } = usePromptsView(
    state,
    query,
    isBusy,
  );

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
      onDiscard={discard}
    >
      <div className="space-y-6">
        <PromptConflictNotice />
        <PromptMentionHelp />
        <PromptSyncSection />
        <SettingsGroup
          title={t("settings:promptsCustomHeading")}
          contentClassName="space-y-6 divide-y-0"
          action={
            <PromptCreateButton
              onClick={startCreate}
              disabled={isBusy || Boolean(editingId) || showCreate}
            />
          }
        >
          {showCreate && (
            <PromptCreateForm
              formState={formState}
              onFormChange={(patch) => setFormState((prev) => ({ ...prev, ...patch }))}
              onCancel={resetForm}
              isBusy={isBusy}
            />
          )}

          <div className="space-y-3">
            <PromptListSearch query={query} onQueryChange={setQuery} disabled={searchDisabled} />
            <PromptListContent
              promptsLoaded={promptsLoaded || prompts.length > 0}
              prompts={visiblePrompts}
              hasPrompts={prompts.length > 0}
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
              deleteTargetId={deleteTargetId}
              pending={pending}
              onChoose={choose}
            />
          </div>
        </SettingsGroup>
      </div>
    </SettingsPageTemplate>
  );
}
