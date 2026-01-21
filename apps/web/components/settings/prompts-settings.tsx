'use client';

import { useCallback, useMemo, useRef, useState, useEffect } from 'react';
import { IconEdit, IconTrash } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@kandev/ui/dialog';
import { Input } from '@kandev/ui/input';
import { Textarea } from '@kandev/ui/textarea';
import { SettingsPageTemplate } from '@/components/settings/settings-page-template';
import { useCustomPrompts } from '@/hooks/use-custom-prompts';
import { useAppStore } from '@/components/state-provider';
import { createPrompt, deletePrompt, updatePrompt } from '@/lib/http';
import { useRequest } from '@/lib/http/use-request';
import type { CustomPrompt } from '@/lib/types/http';

const defaultFormState = {
  name: '',
  content: '',
};

export function PromptsSettings() {
  const { loaded: promptsLoaded } = useCustomPrompts();
  const prompts = useAppStore((state) => state.prompts.items);
  const setPrompts = useAppStore((state) => state.setPrompts);

  const [editingId, setEditingId] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [formState, setFormState] = useState(defaultFormState);
  const editingRef = useRef<HTMLDivElement | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<CustomPrompt | null>(null);

  const isEditing = Boolean(editingId);

  const resetForm = useCallback(() => {
    setEditingId(null);
    setShowCreate(false);
    setFormState(defaultFormState);
  }, []);

  const applyPrompts = useCallback(
    (next: CustomPrompt[]) => {
      const sorted = [...next].sort((a, b) => a.name.localeCompare(b.name));
      setPrompts(sorted);
    },
    [setPrompts]
  );

  const createRequest = useRequest(async (state: typeof defaultFormState) => {
    const payload = {
      name: state.name.trim(),
      content: state.content.trim(),
    };
    const prompt = await createPrompt(payload, { cache: 'no-store' });
    applyPrompts([...prompts, prompt]);
    resetForm();
  });

  const updateRequest = useRequest(async (id: string, state: typeof defaultFormState) => {
    const payload = {
      name: state.name.trim(),
      content: state.content.trim(),
    };
    const updated = await updatePrompt(id, payload, { cache: 'no-store' });
    applyPrompts(prompts.map((prompt) => (prompt.id === id ? updated : prompt)));
    resetForm();
  });

  const deleteRequest = useRequest(async (id: string) => {
    await deletePrompt(id, { cache: 'no-store' });
    applyPrompts(prompts.filter((prompt) => prompt.id !== id));
    if (editingId === id) {
      resetForm();
    }
  });

  const isValid = useMemo(() => {
    return Boolean(formState.name.trim() && formState.content.trim());
  }, [formState]);

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

  const getPromptPreview = (content: string) => {
    return content.split(/\r?\n/)[0] ?? '';
  };

  useEffect(() => {
    if (!editingId) return;
    editingRef.current?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }, [editingId]);

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
          <div className="rounded-lg border border-border/70 bg-background p-4 space-y-3">
            <div className="text-sm font-medium text-foreground">Add prompt</div>
            <Input
              value={formState.name}
              onChange={(event) => setFormState((prev) => ({ ...prev, name: event.target.value }))}
              placeholder="Prompt name"
            />
            <Textarea
              value={formState.content}
              onChange={(event) => setFormState((prev) => ({ ...prev, content: event.target.value }))}
              placeholder="Prompt content"
              rows={5}
              className="resize-y max-h-60 overflow-auto"
            />
            <div className="flex items-center gap-2">
              <Button onClick={handleCreate} disabled={!isValid || isBusy}>
                Add prompt
              </Button>
              <Button variant="ghost" onClick={resetForm} disabled={isBusy}>
                Cancel
              </Button>
            </div>
          </div>
        )}

        <div className="space-y-3">
          {!promptsLoaded ? (
            <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
              Loading prompts…
            </div>
          ) : prompts.length === 0 ? (
            <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
              No prompts yet. Add your first prompt to get started.
            </div>
          ) : (
            prompts.map((prompt) => (
              <div
                key={prompt.id}
                className="rounded-lg border border-border/70 bg-background p-4 flex flex-col gap-3"
                ref={editingId === prompt.id ? editingRef : null}
              >
                <div className="flex items-center justify-between gap-2">
                  <div>
                    <div className="text-sm font-medium text-foreground">@{prompt.name}</div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => startEditing(prompt)}
                      disabled={isBusy || showCreate}
                      className="cursor-pointer"
                    >
                      <IconEdit className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => openDeleteDialog(prompt)}
                      disabled={isBusy}
                      className="cursor-pointer"
                    >
                      <IconTrash className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                {editingId === prompt.id ? (
                  <div className="space-y-3">
                    <Input
                      value={formState.name}
                      onChange={(event) =>
                        setFormState((prev) => ({ ...prev, name: event.target.value }))
                      }
                      placeholder="Prompt name"
                    />
                    <Textarea
                      value={formState.content}
                      onChange={(event) =>
                        setFormState((prev) => ({ ...prev, content: event.target.value }))
                      }
                      placeholder="Prompt content"
                      rows={5}
                      className="resize-y max-h-60 overflow-auto"
                    />
                    <div className="flex items-center gap-2">
                      <Button onClick={handleUpdate} disabled={!isValid || isBusy}>
                        Save changes
                      </Button>
                      <Button variant="ghost" onClick={resetForm} disabled={isBusy}>
                        Cancel
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="text-xs text-muted-foreground truncate">
                    {getPromptPreview(prompt.content)}
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      </div>
      <Dialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => {
          if (!open) {
            closeDeleteDialog();
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete prompt</DialogTitle>
            <DialogDescription>
              This will permanently remove{' '}
              <span className="font-medium text-foreground">
                {deleteTarget ? `@${deleteTarget.name}` : 'this prompt'}
              </span>
              . This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={closeDeleteDialog}>
              Cancel
            </Button>
            <Button type="button" variant="destructive" onClick={confirmDelete} disabled={isBusy}>
              Delete prompt
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsPageTemplate>
  );
}
