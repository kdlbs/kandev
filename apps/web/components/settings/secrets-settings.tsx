"use client";

import { useCallback, useMemo, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { IconEdit, IconTrash, IconEye, IconEyeOff, IconKey } from "@tabler/icons-react";
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
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import {
  createSecret,
  updateSecret,
  deleteSecret,
  revealSecret,
} from "@/lib/api/domains/secrets-api";
import { useRequest } from "@/lib/http/use-request";
import type { SecretListItem, SecretScope, UpdateSecretRequest } from "@/lib/types/http-secrets";
import { WorkspaceSectionHeader } from "@/components/settings/workspaces/workspace-section-header";

export type SecretFormState = {
  name: string;
  value: string;
};

const defaultFormState: SecretFormState = {
  name: "",
  value: "",
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
  showSubmit?: boolean;
  baselineState?: SecretFormState;
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
  showSubmit = true,
  baselineState,
}: SecretFormProps) {
  const { t } = useTranslation();
  const nameIsDirty = Boolean(baselineState) && formState.name.trim() !== baselineState?.name;
  const valueIsDirty = Boolean(baselineState) && formState.value !== baselineState?.value;
  return (
    <div
      className="rounded-lg border border-border/70 bg-background p-4 space-y-3"
      data-settings-dirty={nameIsDirty || valueIsDirty}
    >
      <div className="text-sm font-medium text-foreground">{title}</div>
      <div className="space-y-2">
        <Input
          value={formState.name}
          onChange={(e) => onFormChange({ name: e.target.value })}
          placeholder={t("settings:nameEGOpenaiProductionKey")}
          disabled={isBusy}
          data-settings-dirty={nameIsDirty}
        />
        <Textarea
          value={formState.value}
          onChange={(e) => onFormChange({ value: e.target.value })}
          placeholder={t("settings:secretValue")}
          rows={2}
          className="resize-y font-mono text-sm"
          disabled={isBusy}
          data-settings-dirty={valueIsDirty}
        />
      </div>
      <div className="flex items-center gap-2">
        {showSubmit && (
          <Button onClick={onSubmit} disabled={!isValid || isBusy} className="cursor-pointer">
            {submitLabel}
          </Button>
        )}
        <Button variant="ghost" onClick={onCancel} disabled={isBusy} className="cursor-pointer">
          {t("settings:cancel")}
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
  workspaceId?: string;
  onEdit: (secret: SecretListItem) => void;
  onDelete: (secret: SecretListItem) => void;
  isBusy: boolean;
  showCreate: boolean;
  isEditing: boolean;
};

function SecretListItemRow({
  secret,
  workspaceId,
  onEdit,
  onDelete,
  isBusy,
  showCreate,
  isEditing,
}: SecretListItemRowProps) {
  const { t } = useTranslation();
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
      const resp = await revealSecret(secret.id, {
        cache: "no-store",
        ...(workspaceId ? { workspaceId } : {}),
      });
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
          <Badge variant="outline" className="shrink-0 text-[10px]">
            {secret.scope === "workspace"
              ? t("settings:workspaceScope")
              : t("settings:globalScope")}
          </Badge>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <Button
            variant="ghost"
            size="icon"
            onClick={handleReveal}
            disabled={revealing || isBusy}
            className="min-h-11 min-w-11 cursor-pointer"
            aria-label={
              revealed
                ? t("settings:hideSecretNamed", { name: secret.name })
                : t("settings:revealSecretNamed", { name: secret.name })
            }
          >
            {revealed ? <IconEyeOff className="h-4 w-4" /> : <IconEye className="h-4 w-4" />}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onEdit(secret)}
            disabled={isBusy || showCreate || isEditing}
            className="min-h-11 min-w-11 cursor-pointer"
            aria-label={t("settings:editSecretNamed", { name: secret.name })}
          >
            <IconEdit className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onDelete(secret)}
            disabled={isBusy}
            className="min-h-11 min-w-11 cursor-pointer"
            aria-label={t("settings:deleteSecretNamed", { name: secret.name })}
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        </div>
      </div>
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

export function DeleteSecretDialog({
  target,
  onClose,
  onConfirm,
  isBusy,
}: DeleteSecretDialogProps) {
  const { t } = useTranslation();
  // The secret's own name is user data: it is interpolated as a value and never
  // routed through a catalog key.
  const name = target?.name ?? t("settings:thisSecret");
  return (
    <Dialog
      open={Boolean(target)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("settings:deleteSecret")}</DialogTitle>
          <DialogDescription>
            <Trans i18nKey="settings:thisWillPermanentlyRemoveSecret" values={{ name }}>
              This will permanently remove{" "}
              <span className="font-medium text-foreground">{name}</span>. This action cannot be
              undone.
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose} className="cursor-pointer">
            {t("settings:cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={onConfirm}
            disabled={isBusy}
            className="cursor-pointer"
          >
            {t("settings:deleteSecret")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/* ------------------------------------------------------------------ */
/*  State + actions hooks                                              */
/* ------------------------------------------------------------------ */

function useSecretsState(
  scope: SecretScope,
  workspaceId?: string,
  initialItems?: SecretListItem[],
) {
  const { loaded, items, addSecret, updateSecret, removeSecret } = useSecrets(
    scope,
    workspaceId,
    initialItems,
  );

  const [editingId, setEditingId] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [formState, setFormState] = useState<SecretFormState>(defaultFormState);
  const [deleteTarget, setDeleteTarget] = useState<SecretListItem | null>(null);

  return {
    loaded,
    items,
    addSecret,
    updateSecretInStore: updateSecret,
    removeSecret,
    scope,
    workspaceId,
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
    scope,
    workspaceId,
  } = state;

  const requestOptions = {
    cache: "no-store" as const,
    ...(scope === "workspace" && workspaceId ? { workspaceId } : {}),
  };

  const resetForm = useCallback(() => {
    setEditingId(null);
    setShowCreate(false);
    setFormState(defaultFormState);
  }, [setEditingId, setShowCreate, setFormState]);

  const isValid = useMemo(() => {
    const nameOk = formState.name.trim().length > 0;
    const valueOk = editingId ? true : formState.value.trim().length > 0;
    return nameOk && valueOk;
  }, [formState, editingId]);

  const createRequest = useRequest(async (s: SecretFormState) => {
    const item = await createSecret(
      {
        name: s.name.trim(),
        value: s.value,
        ...(scope === "workspace" && workspaceId
          ? { scope: "workspace" as const, workspace_id: workspaceId }
          : { scope: "global" as const }),
      },
      requestOptions,
    );
    addToStore(item);
    resetForm();
  });

  const updateRequest = useRequest(async (id: string, s: SecretFormState) => {
    const payload: UpdateSecretRequest = {
      name: s.name.trim(),
    };
    if (s.value.trim()) payload.value = s.value;
    const item = await updateSecret(id, payload, requestOptions);
    updateSecretInStore(item);
    resetForm();
  });

  const deleteRequest = useRequest(async (id: string) => {
    await deleteSecret(id, requestOptions);
    removeFromStore(id);
    if (editingId === id) resetForm();
  });

  const isBusy = createRequest.isLoading || updateRequest.isLoading || deleteRequest.isLoading;

  return {
    resetForm,
    isValid,
    isBusy,
    handleCreate: async () => {
      if (!isValid || isBusy) return;
      await createRequest.run(formState);
    },
    handleUpdate: async () => {
      if (!isValid || isBusy || !editingId) return;
      await updateRequest.run(editingId, formState);
    },
    startEditing: (secret: SecretListItem) => {
      setEditingId(secret.id);
      setShowCreate(false);
      setFormState({
        name: secret.name,
        value: "",
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
    items: state.items,
  };
}

/* ------------------------------------------------------------------ */
/*  Main component                                                     */
/* ------------------------------------------------------------------ */

function getSecretEditState(
  items: SecretListItem[],
  editingId: string | null,
  formState: SecretFormState,
) {
  const editingSecret = items.find((secret) => secret.id === editingId);
  const revision = JSON.stringify({ ...formState, name: formState.name.trim() });
  const savedRevision = JSON.stringify({ name: editingSecret?.name ?? "", value: "" });
  return {
    revision,
    isDirty: Boolean(editingId) && revision !== savedRevision,
    baseline: { name: editingSecret?.name ?? "", value: "" },
  };
}

export function getSecretDraftMeta(
  items: SecretListItem[],
  editingId: string | null,
  showCreate: boolean,
  formState: SecretFormState,
) {
  const edit = getSecretEditState(items, editingId, formState);
  return {
    edit,
    isDirty: showCreate || edit.isDirty,
    revision: `${showCreate ? "new" : (editingId ?? "none")}:${edit.revision}`,
  };
}

type SecretsSettingsProps = {
  scope?: SecretScope;
  workspaceId?: string;
  initialItems?: SecretListItem[];
};

type SecretsSettingsState = ReturnType<typeof useSecretsState>;
type SecretsSettingsActions = ReturnType<typeof useSecretsActions>;

function secretScopeTitle(t: TFunction, scope: SecretScope) {
  return scope === "workspace" ? t("settings:secrets") : t("settings:globalSecrets");
}

function secretScopeDescription(t: TFunction, scope: SecretScope) {
  return scope === "workspace"
    ? t("settings:workspaceSecretsDescription")
    : t("settings:manageApiKeysAndCredentialsSecrets");
}

function secretDraftInvalidReason(
  t: TFunction,
  showCreate: boolean,
  isDirty: boolean,
  isValid: boolean,
) {
  if (!isDirty || isValid) return undefined;
  return showCreate
    ? t("settings:aSecretNameAndValueAreRequired")
    : t("settings:aSecretNameIsRequired");
}

type SecretsSettingsBodyProps = {
  scope: SecretScope;
  workspaceId?: string;
  state: SecretsSettingsState;
  actions: SecretsSettingsActions;
  edit: ReturnType<typeof getSecretEditState>;
  onFormChange: (patch: Partial<SecretFormState>) => void;
};

function SecretsSettingsBody({
  scope,
  workspaceId,
  state,
  actions,
  edit,
  onFormChange,
}: SecretsSettingsBodyProps) {
  const { t } = useTranslation();
  const { loaded, editingId, showCreate, formState, deleteTarget, items } = state;
  const { isValid, isBusy } = actions;
  return (
    <>
      {/* `secrets-settings-body` is the pseudo-coverage oracle's render anchor for
          this screen (e2e/tests/i18n/pseudo-coverage.spec.ts). Renaming it makes
          that spec time out rather than silently scan an unrendered route. */}
      <div className="space-y-6" data-testid="secrets-settings-body">
        <div className="flex items-center justify-between">
          <div className="text-sm font-medium text-foreground">{secretScopeTitle(t, scope)}</div>
          <Button
            onClick={actions.startCreate}
            disabled={isBusy || Boolean(editingId) || showCreate}
            className="cursor-pointer"
          >
            {t("settings:addSecret")}
          </Button>
        </div>

        {showCreate && (
          <SecretForm
            title={t("settings:newSecret")}
            formState={formState}
            onFormChange={onFormChange}
            onSubmit={actions.handleCreate}
            onCancel={actions.resetForm}
            isValid={isValid}
            isBusy={isBusy}
            submitLabel={t("settings:addSecret")}
            showSubmit={false}
            baselineState={defaultFormState}
          />
        )}

        {editingId && (
          <SecretForm
            title={t("settings:editSecret")}
            formState={formState}
            onFormChange={onFormChange}
            onSubmit={actions.handleUpdate}
            onCancel={actions.resetForm}
            isValid={isValid}
            isBusy={isBusy}
            submitLabel={t("settings:saveChanges")}
            showSubmit={false}
            baselineState={edit.baseline}
          />
        )}

        <div className="space-y-3">
          {!loaded && (
            <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
              {t("settings:loadingSecrets")}
            </div>
          )}
          {loaded && items.length === 0 && !showCreate && (
            <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
              {t("settings:noSecretsYetAddYourFirst")}
            </div>
          )}
          {items.map((secret) => (
            <SecretListItemRow
              key={secret.id}
              secret={secret}
              workspaceId={workspaceId}
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
    </>
  );
}

function SecretsSettingsContent({
  scope,
  workspaceId,
  state,
  actions,
}: {
  scope: SecretScope;
  workspaceId?: string;
  state: SecretsSettingsState;
  actions: SecretsSettingsActions;
}) {
  const { t } = useTranslation();
  const { editingId, showCreate, formState, setFormState, items } = state;
  const { isValid } = actions;
  const { edit, isDirty, revision } = getSecretDraftMeta(items, editingId, showCreate, formState);
  const invalidReason = secretDraftInvalidReason(t, showCreate, isDirty, isValid);

  const onFormChange = (patch: Partial<SecretFormState>) =>
    setFormState((prev) => ({ ...prev, ...patch }));

  return (
    <SettingsPageTemplate
      title={secretScopeTitle(t, scope)}
      description={secretScopeDescription(t, scope)}
      // Workspace-scoped, this is one of six tabs and heads itself like the
      // other five. Install-wide it is a settings page and keeps the page title.
      header={
        scope === "workspace" ? (
          <WorkspaceSectionHeader tab="secrets" description={secretScopeDescription(t, scope)} />
        ) : undefined
      }
      isDirty={isDirty}
      saveStatus="idle"
      saveId={`secrets-${scope}-item-draft`}
      saveRevision={revision}
      canSave={!isDirty || isValid}
      invalidReason={invalidReason}
      onSave={showCreate ? actions.handleCreate : actions.handleUpdate}
      onDiscard={actions.resetForm}
    >
      <SecretsSettingsBody
        scope={scope}
        workspaceId={workspaceId}
        state={state}
        actions={actions}
        edit={edit}
        onFormChange={onFormChange}
      />
    </SettingsPageTemplate>
  );
}

export function SecretsSettings({
  scope = "global",
  workspaceId,
  initialItems,
}: SecretsSettingsProps) {
  const state = useSecretsState(scope, workspaceId, initialItems);
  const actions = useSecretsActions(state);
  return (
    <SecretsSettingsContent
      scope={scope}
      workspaceId={workspaceId}
      state={state}
      actions={actions}
    />
  );
}
