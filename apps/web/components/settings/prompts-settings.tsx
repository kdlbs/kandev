"use client";

import { useCallback, useMemo, useRef, useState, useEffect } from "react";
import { IconEdit, IconTrash, IconLock } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { useAppStore } from "@/components/state-provider";
import { createPrompt, deletePrompt, updatePrompt } from "@/lib/api";
import { useRequest } from "@/lib/http/use-request";
import type { CustomPrompt } from "@/lib/types/http";

const defaultFormState = {
  name: "",
  content: "",
};

type PromptFormState = typeof defaultFormState;

type PromptCreateFormProps = {
  formState: PromptFormState;
  onFormChange: (patch: Partial<PromptFormState>) => void;
  onSubmit: () => void;
  onCancel: () => void;
  isValid: boolean;
  isBusy: boolean;
};

function PromptCreateForm({
  formState,
  onFormChange,
  onSubmit,
  onCancel,
  isValid,
  isBusy,
}: PromptCreateFormProps) {
  return (
    <div className="rounded-lg border border-border/70 bg-background p-4 space-y-3">
      <div className="text-sm font-medium text-foreground">Add prompt</div>
      <Input
        value={formState.name}
        onChange={(event) => onFormChange({ name: event.target.value })}
        placeholder="Prompt name"
      />
      <Textarea
        value={formState.content}
        onChange={(event) => onFormChange({ content: event.target.value })}
        placeholder="Prompt content"
        rows={5}
        className="resize-y max-h-60 overflow-auto"
      />
      <div className="flex items-center gap-2">
        <Button onClick={onSubmit} disabled={!isValid || isBusy}>
          Add prompt
        </Button>
        <Button variant="ghost" onClick={onCancel} disabled={isBusy}>
          Cancel
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
  onUpdate: () => void;
  onCancel: () => void;
  isValid: boolean;
  isBusy: boolean;
  showCreate: boolean;
};

function PromptListItem({
  prompt,
  isEditing,
  editingRef,
  formState,
  onFormChange,
  onStartEditing,
  onOpenDelete,
  onUpdate,
  onCancel,
  isValid,
  isBusy,
  showCreate,
}: PromptListItemProps) {
  const getPromptPreview = (content: string) => {
    return content.split(/\r?\n/)[0] ?? "";
  };

  return (
    <div
      className="rounded-lg border border-border/70 bg-background p-4 flex flex-col gap-3"
      ref={isEditing ? editingRef : null}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <div className="text-sm font-medium text-foreground">@{prompt.name}</div>
          {prompt.builtin && (
            <Badge variant="secondary" className="text-xs">
              <IconLock className="h-3 w-3 mr-1" />
              Built-in
            </Badge>
          )}
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onStartEditing(prompt)}
            disabled={isBusy || showCreate}
            className="cursor-pointer"
          >
            <IconEdit className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onOpenDelete(prompt)}
            disabled={isBusy}
            className="cursor-pointer"
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        </div>
      </div>
      {isEditing ? (
        <div className="space-y-3">
          <Input
            value={formState.name}
            onChange={(event) => onFormChange({ name: event.target.value })}
            placeholder="Prompt name"
          />
          <Textarea
            value={formState.content}
            onChange={(event) => onFormChange({ content: event.target.value })}
            placeholder="Prompt content"
            rows={5}
            className="resize-y max-h-60 overflow-auto"
          />
          <div className="flex items-center gap-2">
            <Button onClick={onUpdate} disabled={!isValid || isBusy}>
              Save changes
            </Button>
            <Button variant="ghost" onClick={onCancel} disabled={isBusy}>
              Cancel
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
  onUpdate: () => void;
  onCancel: () => void;
  isValid: boolean;
  isBusy: boolean;
  showCreate: boolean;
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
  onUpdate,
  onCancel,
  isValid,
  isBusy,
  showCreate,
}: PromptListContentProps) {
  if (!promptsLoaded) {
    return (
      <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
        Loading prompts…
      </div>
    );
  }
  if (prompts.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
        No prompts yet. Add your first prompt to get started.
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
          onUpdate={onUpdate}
          onCancel={onCancel}
          isValid={isValid}
          isBusy={isBusy}
          showCreate={showCreate}
        />
      ))}
    </>
  );
}

type DeletePromptDialogProps = {
  deleteTarget: CustomPrompt | null;
  onClose: () => void;
  onConfirm: () => void;
  isBusy: boolean;
};

function DeletePromptDialog({ deleteTarget, onClose, onConfirm, isBusy }: DeletePromptDialogProps) {
  return (
    <Dialog
      open={Boolean(deleteTarget)}
      onOpenChange={(open) => {
        if (!open) {
          onClose();
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete prompt</DialogTitle>
          <DialogDescription>
            This will permanently remove{" "}
            <span className="font-medium text-foreground">
              {deleteTarget ? `@${deleteTarget.name}` : "this prompt"}
            </span>
            . This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="button" variant="destructive" onClick={onConfirm} disabled={isBusy}>
            Delete prompt
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
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

  const resetForm = useCallback(() => {
    setEditingId(null);
    setShowCreate(false);
    setFormState(defaultFormState);
  }, [setEditingId, setShowCreate, setFormState]);

  const applyPrompts = useCallback(
    (next: CustomPrompt[]) => {
      setPrompts([...next].sort((a, b) => a.name.localeCompare(b.name)));
    },
    [setPrompts],
  );

  const isValid = useMemo(
    () => Boolean(formState.name.trim() && formState.content.trim()),
    [formState],
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

  const isBusy = createRequest.isLoading || updateRequest.isLoading || deleteRequest.isLoading;
  const handleCreate = () => {
    if (!isValid || isBusy) return;
    createRequest.run(formState).catch(() => undefined);
  };
  const handleUpdate = () => {
    if (!isValid || isBusy || !editingId) return;
    updateRequest.run(editingId, formState).catch(() => undefined);
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
    if (!deleteTarget) return;
    deleteRequest.run(deleteTarget.id).catch(() => undefined);
    closeDeleteDialog();
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

export function PromptsSettings() {
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

  useEffect(() => {
    if (!editingId) return;
    editingRef.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [editingId, editingRef]);

  return (
    <SettingsPageTemplate
      title="Prompts"
      description="Create reusable prompt snippets for the chat input."
      isDirty={false}
      saveStatus="idle"
      onSave={() => undefined}
      showSaveButton={false}
    >
      <div className="rounded-lg border border-border/70 bg-muted/30 p-4 text-xs text-muted-foreground">
        Use <span className="font-medium text-foreground">@name</span> in the chat input to insert a
        prompt’s content. Prompts are matched by name and expanded in place.
      </div>
      <div className="space-y-6 mt-4">
        <div className="flex items-center justify-between">
          <div className="text-sm font-medium text-foreground">Custom prompts</div>
          <Button onClick={startCreate} disabled={isBusy || isEditing || showCreate}>
            Add prompt
          </Button>
        </div>

        {showCreate && (
          <PromptCreateForm
            formState={formState}
            onFormChange={(patch) => setFormState((prev) => ({ ...prev, ...patch }))}
            onSubmit={handleCreate}
            onCancel={resetForm}
            isValid={isValid}
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
            onUpdate={handleUpdate}
            onCancel={resetForm}
            isValid={isValid}
            isBusy={isBusy}
            showCreate={showCreate}
          />
        </div>
      </div>
      <DeletePromptDialog
        deleteTarget={deleteTarget}
        onClose={closeDeleteDialog}
        onConfirm={confirmDelete}
        isBusy={isBusy}
      />
    </SettingsPageTemplate>
  );
}
