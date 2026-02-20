"use client";

import { useCallback, useMemo, useState } from "react";
import { IconEdit, IconTrash, IconEye, IconEyeOff, IconKey } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import { useAppStore } from "@/components/state-provider";
import {
  createSecret,
  updateSecret,
  deleteSecret,
  revealSecret,
} from "@/lib/api/domains/secrets-api";
import { useRequest } from "@/lib/http/use-request";
import type { SecretListItem, SecretCategory } from "@/lib/types/http-secrets";

const CATEGORY_OPTIONS: Array<{ value: SecretCategory; label: string }> = [
  { value: "api_key", label: "API Key" },
  { value: "service_token", label: "Service Token" },
  { value: "ssh_key", label: "SSH Key" },
  { value: "custom", label: "Custom" },
];

const CATEGORY_LABELS: Record<SecretCategory, string> = {
  api_key: "API Key",
  service_token: "Service Token",
  ssh_key: "SSH Key",
  custom: "Custom",
};

type SecretFormState = {
  name: string;
  envKey: string;
  value: string;
  category: SecretCategory;
};

const defaultFormState: SecretFormState = {
  name: "",
  envKey: "",
  value: "",
  category: "api_key",
};

/* ------------------------------------------------------------------ */
/*  Create / Edit form                                                 */
/* ------------------------------------------------------------------ */

type SecretFormProps = {
  title: string;
  formState: SecretFormState;
  onFormChange: (patch: Partial<SecretFormState>) => void;
  onSubmit: () => void;
  onCancel: () => void;
  isValid: boolean;
  isBusy: boolean;
  submitLabel: string;
  hideEnvKey?: boolean;
};

function SecretForm({
  title,
  formState,
  onFormChange,
  onSubmit,
  onCancel,
  isValid,
  isBusy,
  submitLabel,
  hideEnvKey,
}: SecretFormProps) {
  return (
    <div className="rounded-lg border border-border/70 bg-background p-4 space-y-3">
      <div className="text-sm font-medium text-foreground">{title}</div>
      <div className="space-y-2">
        <Input
          value={formState.name}
          onChange={(e) => onFormChange({ name: e.target.value })}
          placeholder="Display name (e.g. OpenAI Production)"
        />
        {!hideEnvKey && (
          <Input
            value={formState.envKey}
            onChange={(e) => onFormChange({ envKey: e.target.value.toUpperCase() })}
            placeholder="ENV key (e.g. OPENAI_API_KEY)"
            className="font-mono text-sm"
          />
        )}
        <Textarea
          value={formState.value}
          onChange={(e) => onFormChange({ value: e.target.value })}
          placeholder="Secret value"
          rows={2}
          className="resize-y font-mono text-sm"
        />
        <Select
          value={formState.category}
          onValueChange={(v) => onFormChange({ category: v as SecretCategory })}
        >
          <SelectTrigger className="w-[180px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {CATEGORY_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="flex items-center gap-2">
        <Button onClick={onSubmit} disabled={!isValid || isBusy} className="cursor-pointer">
          {submitLabel}
        </Button>
        <Button variant="ghost" onClick={onCancel} disabled={isBusy} className="cursor-pointer">
          Cancel
        </Button>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  List item                                                          */
/* ------------------------------------------------------------------ */

type SecretListItemRowProps = {
  secret: SecretListItem;
  onEdit: (secret: SecretListItem) => void;
  onDelete: (secret: SecretListItem) => void;
  isBusy: boolean;
  showCreate: boolean;
  isEditing: boolean;
};

function SecretListItemRow({
  secret,
  onEdit,
  onDelete,
  isBusy,
  showCreate,
  isEditing,
}: SecretListItemRowProps) {
  const [revealed, setRevealed] = useState(false);
  const [revealedValue, setRevealedValue] = useState<string | null>(null);
  const [revealing, setRevealing] = useState(false);

  const handleReveal = async () => {
    if (revealed) {
      setRevealed(false);
      setRevealedValue(null);
      return;
    }
    setRevealing(true);
    try {
      const resp = await revealSecret(secret.id, { cache: "no-store" });
      setRevealedValue(resp.value);
      setRevealed(true);
    } catch {
      // ignore
    } finally {
      setRevealing(false);
    }
  };

  return (
    <div className="rounded-lg border border-border/70 bg-background p-4 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <IconKey className="h-4 w-4 text-muted-foreground shrink-0" />
          <div className="text-sm font-medium text-foreground truncate">{secret.name}</div>
          <Badge variant="secondary" className="text-[10px] shrink-0">
            {CATEGORY_LABELS[secret.category] ?? secret.category}
          </Badge>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <Button
            variant="ghost"
            size="icon"
            onClick={handleReveal}
            disabled={revealing || isBusy}
            className="cursor-pointer"
          >
            {revealed ? <IconEyeOff className="h-4 w-4" /> : <IconEye className="h-4 w-4" />}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onEdit(secret)}
            disabled={isBusy || showCreate || isEditing}
            className="cursor-pointer"
          >
            <IconEdit className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onDelete(secret)}
            disabled={isBusy}
            className="cursor-pointer"
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        </div>
      </div>
      <div className="text-xs text-muted-foreground font-mono">{secret.env_key}</div>
      {revealed && revealedValue !== null && (
        <div className="text-xs font-mono bg-muted/50 rounded px-2 py-1 break-all">
          {revealedValue}
        </div>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Delete dialog                                                      */
/* ------------------------------------------------------------------ */

type DeleteSecretDialogProps = {
  target: SecretListItem | null;
  onClose: () => void;
  onConfirm: () => void;
  isBusy: boolean;
};

function DeleteSecretDialog({ target, onClose, onConfirm, isBusy }: DeleteSecretDialogProps) {
  return (
    <Dialog
      open={Boolean(target)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete secret</DialogTitle>
          <DialogDescription>
            This will permanently remove{" "}
            <span className="font-medium text-foreground">{target?.name ?? "this secret"}</span>.
            This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose} className="cursor-pointer">
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={onConfirm}
            disabled={isBusy}
            className="cursor-pointer"
          >
            Delete secret
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/* ------------------------------------------------------------------ */
/*  State + actions hooks                                              */
/* ------------------------------------------------------------------ */

function useSecretsState() {
  const { loaded } = useSecrets();
  const items = useAppStore((s) => s.secrets.items);
  const addSecret = useAppStore((s) => s.addSecret);
  const updateSecretInStore = useAppStore((s) => s.updateSecret);
  const removeSecret = useAppStore((s) => s.removeSecret);

  const [editingId, setEditingId] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [formState, setFormState] = useState<SecretFormState>(defaultFormState);
  const [deleteTarget, setDeleteTarget] = useState<SecretListItem | null>(null);

  return {
    loaded,
    items,
    addSecret,
    updateSecretInStore,
    removeSecret,
    editingId,
    setEditingId,
    showCreate,
    setShowCreate,
    formState,
    setFormState,
    deleteTarget,
    setDeleteTarget,
  };
}

function useSecretsActions(state: ReturnType<typeof useSecretsState>) {
  const {
    items,
    addSecret: addToStore,
    updateSecretInStore,
    removeSecret: removeFromStore,
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

  const isValid = useMemo(() => {
    const nameOk = formState.name.trim().length > 0;
    const envKeyOk = /^[A-Z][A-Z0-9_]*$/.test(formState.envKey);
    const valueOk = editingId ? true : formState.value.trim().length > 0;
    return nameOk && envKeyOk && valueOk;
  }, [formState, editingId]);

  const createRequest = useRequest(async (s: SecretFormState) => {
    const item = await createSecret(
      { name: s.name.trim(), env_key: s.envKey.trim(), value: s.value, category: s.category },
      { cache: "no-store" },
    );
    addToStore(item);
    resetForm();
  });

  const updateRequest = useRequest(async (id: string, s: SecretFormState) => {
    const payload: Record<string, unknown> = {
      name: s.name.trim(),
      category: s.category,
    };
    if (s.value.trim()) payload.value = s.value;
    const item = await updateSecret(id, payload, { cache: "no-store" });
    updateSecretInStore(item);
    resetForm();
  });

  const deleteRequest = useRequest(async (id: string) => {
    await deleteSecret(id, { cache: "no-store" });
    removeFromStore(id);
    if (editingId === id) resetForm();
  });

  const isBusy = createRequest.isLoading || updateRequest.isLoading || deleteRequest.isLoading;

  return {
    resetForm,
    isValid,
    isBusy,
    handleCreate: () => {
      if (!isValid || isBusy) return;
      createRequest.run(formState).catch(() => undefined);
    },
    handleUpdate: () => {
      if (!isValid || isBusy || !editingId) return;
      updateRequest.run(editingId, formState).catch(() => undefined);
    },
    startEditing: (secret: SecretListItem) => {
      setEditingId(secret.id);
      setShowCreate(false);
      setFormState({
        name: secret.name,
        envKey: secret.env_key,
        value: "",
        category: secret.category,
      });
    },
    startCreate: () => {
      setEditingId(null);
      setShowCreate(true);
      setFormState(defaultFormState);
    },
    openDelete: (secret: SecretListItem) => setDeleteTarget(secret),
    closeDelete: () => setDeleteTarget(null),
    confirmDelete: () => {
      if (!deleteTarget) return;
      deleteRequest.run(deleteTarget.id).catch(() => undefined);
      setDeleteTarget(null);
    },
    items,
  };
}

/* ------------------------------------------------------------------ */
/*  Main component                                                     */
/* ------------------------------------------------------------------ */

export function SecretsSettings() {
  const state = useSecretsState();
  const { loaded, editingId, showCreate, formState, setFormState, deleteTarget, items } = state;
  const actions = useSecretsActions(state);
  const { isValid, isBusy } = actions;

  const onFormChange = (patch: Partial<SecretFormState>) =>
    setFormState((prev) => ({ ...prev, ...patch }));

  return (
    <SettingsPageTemplate
      title="Secrets"
      description="Manage API keys and credentials. Secrets are encrypted at rest and injected into agent environments."
      isDirty={false}
      saveStatus="idle"
      onSave={() => undefined}
      showSaveButton={false}
    >
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div className="text-sm font-medium text-foreground">Secrets</div>
          <Button
            onClick={actions.startCreate}
            disabled={isBusy || Boolean(editingId) || showCreate}
            className="cursor-pointer"
          >
            Add secret
          </Button>
        </div>

        {showCreate && (
          <SecretForm
            title="New secret"
            formState={formState}
            onFormChange={onFormChange}
            onSubmit={actions.handleCreate}
            onCancel={actions.resetForm}
            isValid={isValid}
            isBusy={isBusy}
            submitLabel="Add secret"
          />
        )}

        {editingId && (
          <SecretForm
            title="Edit secret"
            formState={formState}
            onFormChange={onFormChange}
            onSubmit={actions.handleUpdate}
            onCancel={actions.resetForm}
            isValid={isValid}
            isBusy={isBusy}
            submitLabel="Save changes"
            hideEnvKey
          />
        )}

        <div className="space-y-3">
          {!loaded && (
            <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
              Loading secrets...
            </div>
          )}
          {loaded && items.length === 0 && !showCreate && (
            <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
              No secrets yet. Add your first secret to get started.
            </div>
          )}
          {items.map((secret) => (
            <SecretListItemRow
              key={secret.id}
              secret={secret}
              onEdit={actions.startEditing}
              onDelete={actions.openDelete}
              isBusy={isBusy}
              showCreate={showCreate}
              isEditing={editingId === secret.id}
            />
          ))}
        </div>
      </div>

      <DeleteSecretDialog
        target={deleteTarget}
        onClose={actions.closeDelete}
        onConfirm={actions.confirmDelete}
        isBusy={isBusy}
      />
    </SettingsPageTemplate>
  );
}
