"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { Button } from "@kandev/ui/button";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { WorkspaceSectionHeader } from "@/components/settings/workspaces/workspace-section-header";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import type { ApiRequestOptions } from "@/lib/api/client";
import { createSecret, updateSecret, deleteSecret } from "@/lib/api/domains/secrets-api";
import { useRequest } from "@/lib/http/use-request";
import type { SecretListItem, SecretScope, UpdateSecretRequest } from "@/lib/types/http-secrets";
import { SecretForm, defaultFormState, type SecretFormState } from "./secret-form";
import { SecretListItemRow } from "./secrets-list-item-row";
import { CopyMoveSecretDialog, type CopyMoveMode } from "./copy-move-secret-dialog";

/* ------------------------------------------------------------------ */
/*  State + actions hooks                                              */
/* ------------------------------------------------------------------ */

/** Holds the secrets list plus all settings-panel UI state (editing, create form, delete/transfer targets). */
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
  const [transferTarget, setTransferTarget] = useState<SecretListItem | null>(null);
  const [transferOpen, setTransferOpen] = useState(false);

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
    transferTarget,
    setTransferTarget,
    transferOpen,
    setTransferOpen,
  };
}

/**
 * Remembers the focused element when a transfer dialog opens and returns
 * focus to it on close. Radix restores to a stale node when the settings list
 * re-renders mid-close, leaving focus on <body>; this makes the keyboard
 * contract deterministic.
 */
function useTransferFocusRestore() {
  const triggerRef = useRef<HTMLElement | null>(null);
  const rememberTransferTrigger = useCallback(() => {
    triggerRef.current =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;
  }, []);
  const restoreTransferTrigger = useCallback(() => {
    const trigger = triggerRef.current;
    triggerRef.current = null;
    if (trigger) {
      window.setTimeout(() => {
        if (document.contains(trigger)) {
          trigger.focus();
        }
      }, 0);
    }
  }, []);
  return { rememberTransferTrigger, restoreTransferTrigger };
}

/** Builds the create/update/delete request runners that mutate the store and reset the form on success. */
function useSecretRequests(
  scope: SecretScope,
  workspaceId: string | undefined,
  requestOptions: ApiRequestOptions,
  deps: {
    addToStore: (item: SecretListItem) => void;
    updateSecretInStore: (item: SecretListItem) => void;
    removeFromStore: (id: string) => void;
    resetForm: () => void;
    editingId: string | null;
  },
) {
  const { addToStore, updateSecretInStore, removeFromStore, resetForm, editingId } = deps;
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
  return { createRequest, updateRequest, deleteRequest };
}

/** Composes request runners and UI actions (create, edit, delete, transfer) from the secrets state. */
function useSecretsActions(state: ReturnType<typeof useSecretsState>) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { rememberTransferTrigger, restoreTransferTrigger } = useTransferFocusRestore();
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
    setTransferTarget,
    setTransferOpen,
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

  const isValid = useMemo(
    () =>
      formState.name.trim().length > 0 && (editingId ? true : formState.value.trim().length > 0),
    [formState, editingId],
  );
  const { createRequest, updateRequest, deleteRequest } = useSecretRequests(
    scope,
    workspaceId,
    requestOptions,
    {
      addToStore,
      updateSecretInStore,
      removeFromStore,
      resetForm,
      editingId,
    },
  );
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
      const id = deleteTarget.id;
      setDeleteTarget(null);
      return deleteRequest.run(id).catch(() => {
        toast({ description: t("settings:secretDeleteFailed"), variant: "error" });
      });
    },
    openTransfer: (secret: SecretListItem) => {
      // Remember the focused Copy/Move button; close restores it because
      // Radix targets a stale node when the list re-renders mid-close.
      rememberTransferTrigger();
      setTransferTarget(secret);
      setTransferOpen(true);
    },
    closeTransfer: () => {
      setTransferOpen(false);
      restoreTransferTrigger();
    },
    items: state.items,
  };
}

/* ------------------------------------------------------------------ */
/*  Main component                                                     */
/* ------------------------------------------------------------------ */

/** Computes the edit draft's revision, dirty flag, and baseline from the current form state. */
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

/** Computes the combined draft metadata (dirty flag, revision) covering both create and edit states. */
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

export type SecretsSettingsState = {
  loaded: boolean;
  items: SecretListItem[];
  addSecret: (item: SecretListItem) => void;
  updateSecretInStore: (item: SecretListItem) => void;
  removeSecret: (id: string) => void;
  scope: SecretScope;
  workspaceId?: string;
  editingId: string | null;
  setEditingId: (id: string | null) => void;
  showCreate: boolean;
  setShowCreate: (show: boolean) => void;
  formState: SecretFormState;
  setFormState: (state: SecretFormState | ((prev: SecretFormState) => SecretFormState)) => void;
  deleteTarget: SecretListItem | null;
  setDeleteTarget: (target: SecretListItem | null) => void;
  transferTarget: SecretListItem | null;
  setTransferTarget: (target: SecretListItem | null) => void;
  transferOpen: boolean;
  setTransferOpen: (open: boolean) => void;
};

export type SecretsSettingsActions = {
  resetForm: () => void;
  isValid: boolean;
  isBusy: boolean;
  handleCreate: () => Promise<void>;
  handleUpdate: () => Promise<void>;
  startEditing: (secret: SecretListItem) => void;
  startCreate: () => void;
  openDelete: (secret: SecretListItem) => void;
  closeDelete: () => void;
  confirmDelete: () => void | Promise<void>;
  openTransfer: (secret: SecretListItem) => void;
  closeTransfer: () => void;
  items: SecretListItem[];
};

type SecretsSettingsProps = {
  scope?: SecretScope;
  workspaceId?: string;
  initialItems?: SecretListItem[];
};

/** Returns the settings title for the given secret scope. */
function secretScopeTitle(t: TFunction, scope: SecretScope) {
  return scope === "workspace" ? t("settings:secrets") : t("settings:globalSecrets");
}

/** Returns the settings description for the given secret scope. */
function secretScopeDescription(t: TFunction, scope: SecretScope) {
  return scope === "workspace"
    ? t("settings:workspaceSecretsDescription")
    : t("settings:manageApiKeysAndCredentialsSecrets");
}

/** Returns the reason a dirty draft cannot be saved, or undefined when it can. */
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

/** Renders the secrets list, create/edit forms, and delete dialog for the current scope. */
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
              onDeleteCancel={actions.closeDelete}
              onDeleteConfirm={actions.confirmDelete}
              onCopyMove={actions.openTransfer}
              isBusy={isBusy}
              showCreate={showCreate}
              isEditing={editingId === secret.id}
              isDeleteConfirming={deleteTarget?.id === secret.id}
            />
          ))}
        </div>
      </div>
    </>
  );
}

/** Renders the page-template wrapper with dirty-state handling for the secrets panel. */
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

  /** Applies a partial form patch to the draft state. */
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

/** Renders the full secrets settings panel: state, actions, and the copy/move dialog. */
export function SecretsSettings({
  scope = "global",
  workspaceId,
  initialItems,
}: SecretsSettingsProps) {
  const { t } = useTranslation();
  const state = useSecretsState(scope, workspaceId, initialItems);
  const actions = useSecretsActions(state);
  const globalAdd = useAppStore((s) => s.addSecret);
  const workspaceNames = useAppStore((s) => s.workspaces.items);

  /** Resolves the display origin token for a secret's source scope. */
  const originTokenFor = (secret: SecretListItem) =>
    secret.scope === "workspace"
      ? (workspaceNames.find((workspace) => workspace.id === secret.workspace_id)?.name ??
        secret.workspace_id ??
        "workspace")
      : t("settings:globalScope");

  // Route the transfer result by the RETURNED item's scope, never the page
  // scope: Global targets always land in the Global store (from any page),
  // workspace targets join the page's list only when the page is that
  // workspace, and a Move removes the source from the page's own list.
  /** Routes a completed transfer: adds the returned item to the right store and removes the source on move. */
  const handleTransferCompleted = (item: SecretListItem, mode: CopyMoveMode) => {
    if (!item.scope || item.scope === "global") {
      globalAdd(item);
    } else if (scope === "workspace" && workspaceId === item.workspace_id) {
      state.addSecret(item);
    }
    if (mode === "move" && state.transferTarget) {
      state.removeSecret(state.transferTarget.id);
    }
    actions.closeTransfer();
  };

  return (
    <>
      <SecretsSettingsContent
        scope={scope}
        workspaceId={workspaceId}
        state={state}
        actions={actions}
      />
      {state.transferTarget && (
        <CopyMoveSecretDialog
          secret={state.transferTarget}
          originToken={originTokenFor(state.transferTarget)}
          open={state.transferOpen}
          onClose={actions.closeTransfer}
          onCompleted={handleTransferCompleted}
        />
      )}
    </>
  );
}
