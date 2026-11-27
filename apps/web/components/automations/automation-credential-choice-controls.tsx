"use client";

import { type AnimationEvent, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  grantWorkspaceCredential,
  revokeWorkspaceCredential,
} from "@/lib/api/domains/automation-api";
import type { Automation, AutomationCredentialChoice } from "@/lib/types/automation";
import type { Repository } from "@/lib/types/http";
export { PersonalCredentialFields } from "./automation-personal-credential-controls";

type GrantTarget = {
  repositoryId: string;
  repositoryName: string;
  key: string;
  secretName: string;
  credentialRevision: number;
  creatorName: string;
  bindingUpdatedAt: string;
  workspaceOwnerRevision: number;
};

type WorkspaceCredentialRowData = {
  repository: Repository;
  choice: AutomationCredentialChoice;
};

function focusAfterDrawerClose(event: AnimationEvent<HTMLDivElement>, restoreFocus: () => void) {
  if (event.target === event.currentTarget && event.currentTarget.dataset.state === "closed") {
    restoreFocus();
  }
}

function workspaceCredentialRows(
  automation: Automation | null,
  repositories: Repository[],
  selectedRepositoryIds: string[],
): WorkspaceCredentialRowData[] {
  const selected = new Set(selectedRepositoryIds);
  return (automation?.credential_choices ?? [])
    .filter(
      (choice) =>
        choice.kind === "workspace" && choice.repository_id && selected.has(choice.repository_id),
    )
    .flatMap((choice) => {
      const repository = repositories.find((item) => item.id === choice.repository_id);
      return repository ? [{ repository, choice }] : [];
    });
}

function WorkspaceCredentialAction({
  choice,
  canGrant,
  canRevoke,
  saving,
  onGrant,
  onRevoke,
}: {
  choice: AutomationCredentialChoice;
  canGrant: boolean;
  canRevoke: boolean;
  saving: boolean;
  onGrant: () => void;
  onRevoke: () => void;
}) {
  const { t } = useTranslation();
  if (canRevoke) {
    return (
      <Button
        type="button"
        variant="outline"
        className="min-h-7 max-md:min-h-12 [@media(any-pointer:coarse)]:min-h-12 w-full sm:w-auto"
        disabled={saving}
        onClick={onRevoke}
        data-testid={`automation-workspace-credential-revoke-${choice.key}`}
      >
        {t("automations:credentialRevokeWorkspaceSecret")}
      </Button>
    );
  }
  if (!canGrant) return null;
  return (
    <Button
      type="button"
      variant="outline"
      className="min-h-7 max-md:min-h-12 [@media(any-pointer:coarse)]:min-h-12 w-full sm:w-auto"
      disabled={saving}
      onClick={onGrant}
      data-testid={`automation-workspace-credential-grant-${choice.key}`}
    >
      {t("automations:credentialGrantWorkspaceSecret")}
    </Button>
  );
}

function WorkspaceCredentialRow({
  repository,
  choice,
  isWorkspaceOwner,
  canRevoke,
  canGrant,
  saving,
  onRef,
  onGrant,
  onRevoke,
}: {
  repository: Repository;
  choice: AutomationCredentialChoice;
  isWorkspaceOwner: boolean;
  canRevoke: boolean;
  canGrant: boolean;
  saving: boolean;
  onRef: (element: HTMLDivElement | null) => void;
  onGrant: () => void;
  onRevoke: () => void;
}) {
  const { t } = useTranslation();
  const actionCanRevoke = isWorkspaceOwner && canRevoke;
  const actionCanGrant = isWorkspaceOwner && canGrant;
  let status: string;
  switch (choice.state) {
    case "granted":
      status = t("automations:credentialWorkspaceApproved");
      break;
    case "revoked":
      status = t("automations:credentialWorkspaceRevoked");
      break;
    case "stale":
      status = t("automations:credentialWorkspaceStale");
      break;
    default:
      status = t("automations:credentialWorkspaceApprovalNeeded");
  }

  return (
    <div
      ref={onRef}
      tabIndex={-1}
      className="flex min-w-0 flex-col gap-2 rounded-md border border-border/70 p-3 sm:flex-row sm:items-center sm:justify-between"
      data-testid={`automation-workspace-credential-${choice.key}`}
    >
      <div className="min-w-0 space-y-0.5">
        <p className="break-all font-mono text-xs">{choice.key}</p>
        <p className="text-xs text-muted-foreground">
          {t("automations:credentialWorkspaceBindingStatus", {
            repository: repository.name,
            status,
          })}
        </p>
      </div>
      <WorkspaceCredentialAction
        choice={choice}
        canGrant={actionCanGrant}
        canRevoke={actionCanRevoke}
        saving={saving}
        onGrant={onGrant}
        onRevoke={onRevoke}
      />
    </div>
  );
}

function WorkspaceCredentialRows({
  rows,
  isWorkspaceOwner,
  saving,
  actionError,
  canGrant,
  canRevoke,
  onRowRef,
  onGrant,
  onRevoke,
}: {
  rows: WorkspaceCredentialRowData[];
  isWorkspaceOwner: boolean;
  saving: boolean;
  actionError: boolean;
  canGrant: (choice: AutomationCredentialChoice) => boolean;
  canRevoke: (choice: AutomationCredentialChoice) => boolean;
  onRowRef: (key: string, element: HTMLDivElement | null) => void;
  onGrant: (rowKey: string, repository: Repository, choice: AutomationCredentialChoice) => void;
  onRevoke: (repository: Repository, choice: AutomationCredentialChoice) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" data-testid="automation-workspace-credential-choices">
      {rows.map(({ repository, choice }) => {
        const rowKey = `${repository.id}:${choice.key}`;
        return (
          <WorkspaceCredentialRow
            key={rowKey}
            repository={repository}
            choice={choice}
            isWorkspaceOwner={isWorkspaceOwner}
            canGrant={canGrant(choice)}
            canRevoke={canRevoke(choice)}
            saving={saving}
            onRef={(element) => onRowRef(rowKey, element)}
            onGrant={() => onGrant(rowKey, repository, choice)}
            onRevoke={() => onRevoke(repository, choice)}
          />
        );
      })}
      {actionError ? (
        <p role="alert" className="text-sm text-destructive">
          {t("automations:credentialGrantFailed")}
        </p>
      ) : null}
    </div>
  );
}

function WorkspaceCredentialConfirmation({
  isMobile,
  target,
  saving,
  actionError,
  onGrant,
  onCancel,
  onCloseAutoFocus,
  onAnimationEnd,
}: {
  isMobile: boolean;
  target: GrantTarget | null;
  saving: boolean;
  actionError: boolean;
  onGrant: () => void;
  onCancel: () => void;
  onCloseAutoFocus: (event: Event) => void;
  onAnimationEnd: () => void;
}) {
  const { t } = useTranslation();
  const description = target
    ? t("automations:credentialGrantDescription", {
        repository: target.repositoryName,
        key: target.key,
        secret: target.secretName,
        creator: target.creatorName,
      })
    : null;
  const details = (
    <div className={isMobile ? "space-y-3 px-4" : "space-y-3"}>
      <p className="text-sm">{description}</p>
      <p className="text-xs text-muted-foreground">
        {t("automations:credentialGrantRuntimeEffect")}
      </p>
      {actionError ? (
        <p role="alert" className="text-sm text-destructive">
          {t("automations:credentialGrantFailed")}
        </p>
      ) : null}
    </div>
  );
  const footer = isMobile ? (
    <DrawerFooter className="px-4 pb-[calc(1rem+env(safe-area-inset-bottom,0px))]">
      <Button onClick={onGrant} disabled={saving} className="min-h-12 w-full">
        {t("automations:credentialGrantConfirm")}
      </Button>
      <Button variant="outline" disabled={saving} onClick={onCancel} className="min-h-12 w-full">
        {t("common:cancel")}
      </Button>
    </DrawerFooter>
  ) : (
    <DialogFooter>
      <Button variant="outline" disabled={saving} onClick={onCancel}>
        {t("common:cancel")}
      </Button>
      <Button onClick={onGrant} disabled={saving}>
        {t("automations:credentialGrantConfirm")}
      </Button>
    </DialogFooter>
  );

  if (isMobile) {
    return (
      <Drawer open={target !== null} onOpenChange={(open) => !open && onCancel()}>
        <DrawerContent
          className="max-h-[85dvh] pb-[env(safe-area-inset-bottom)]"
          data-testid="automation-workspace-credential-confirmation-drawer"
          onCloseAutoFocus={onCloseAutoFocus}
          onAnimationEnd={(event) => focusAfterDrawerClose(event, onAnimationEnd)}
        >
          <DrawerHeader className="px-4">
            <DrawerTitle>{t("automations:credentialGrantTitle")}</DrawerTitle>
            <DrawerDescription>{t("automations:credentialGrantTarget")}</DrawerDescription>
          </DrawerHeader>
          {details}
          {footer}
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={target !== null} onOpenChange={(open) => !open && onCancel()}>
      <DialogContent
        data-testid="automation-workspace-credential-confirmation-dialog"
        onCloseAutoFocus={onCloseAutoFocus}
      >
        <DialogHeader>
          <DialogTitle>{t("automations:credentialGrantTitle")}</DialogTitle>
          <DialogDescription>{t("automations:credentialGrantTarget")}</DialogDescription>
        </DialogHeader>
        {details}
        {footer}
      </DialogContent>
    </Dialog>
  );
}

function useWorkspaceCredentialActions(
  automation: Automation | null,
  onAutomationChange: (automation: Automation) => void,
) {
  const [target, setTarget] = useState<GrantTarget | null>(null);
  const [saving, setSaving] = useState(false);
  const [actionError, setActionError] = useState(false);
  const rowRefs = useRef(new Map<string, HTMLDivElement>());
  const returnFocusKey = useRef<string | null>(null);

  const grant = async () => {
    if (!automation?.id || !target) return;
    setSaving(true);
    setActionError(false);
    try {
      const updated = await grantWorkspaceCredential(automation.id, {
        repository_id: target.repositoryId,
        key: target.key,
        expected_automation_revision: automation.resource_revision ?? 0,
        expected_credential_revision: target.credentialRevision,
        expected_binding_updated_at: target.bindingUpdatedAt,
        expected_workspace_owner_revision: target.workspaceOwnerRevision,
      });
      onAutomationChange(updated);
      setTarget(null);
    } catch {
      setActionError(true);
    } finally {
      setSaving(false);
    }
  };
  const revoke = async (repository: Repository, choice: AutomationCredentialChoice) => {
    if (!automation?.id) return;
    setSaving(true);
    setActionError(false);
    try {
      const updated = await revokeWorkspaceCredential(automation.id, {
        repository_id: repository.id,
        key: choice.key,
        expected_automation_revision: automation.resource_revision ?? 0,
        expected_credential_revision: choice.revision ?? 0,
        expected_workspace_owner_revision: choice.workspace_owner_revision ?? 0,
      });
      onAutomationChange(updated);
    } catch {
      setActionError(true);
    } finally {
      setSaving(false);
    }
  };
  const openGrant = (
    rowKey: string,
    repository: Repository,
    choice: AutomationCredentialChoice,
  ) => {
    setActionError(false);
    returnFocusKey.current = rowKey;
    setTarget({
      repositoryId: repository.id,
      repositoryName: repository.name,
      key: choice.key,
      secretName: choice.secret_name ?? "",
      credentialRevision: choice.revision ?? 0,
      creatorName: choice.creator_name ?? "",
      bindingUpdatedAt: choice.binding_updated_at ?? "",
      workspaceOwnerRevision: choice.workspace_owner_revision ?? 0,
    });
  };
  const setRowRef = (key: string, element: HTMLDivElement | null) => {
    if (element) rowRefs.current.set(key, element);
    else rowRefs.current.delete(key);
  };
  const focusRow = () => {
    const key = returnFocusKey.current;
    if (key) rowRefs.current.get(key)?.focus({ preventScroll: true });
  };
  const restoreRowFocus = (event: Event) => {
    event.preventDefault();
    focusRow();
  };
  const restoreRowFocusAfterDrawerClose = () => {
    focusRow();
  };

  return {
    target,
    saving,
    actionError,
    grant,
    revoke,
    openGrant,
    setRowRef,
    restoreRowFocus,
    restoreRowFocusAfterDrawerClose,
    cancelGrant: () => setTarget(null),
    canGrant: (choice: AutomationCredentialChoice) =>
      Boolean(automation?.id) && choice.can_grant === true,
    canRevoke: (choice: AutomationCredentialChoice) =>
      Boolean(automation?.id) && choice.can_revoke === true,
  };
}

export function WorkspaceCredentialChoices({
  automation,
  repositories,
  selectedRepositoryIds,
  isWorkspaceOwner,
  onAutomationChange,
}: {
  automation: Automation | null;
  repositories: Repository[];
  selectedRepositoryIds: string[];
  isWorkspaceOwner: boolean;
  onAutomationChange: (automation: Automation) => void;
}) {
  const { isMobile } = useResponsiveBreakpoint();
  const rows = workspaceCredentialRows(automation, repositories, selectedRepositoryIds);
  const actions = useWorkspaceCredentialActions(automation, onAutomationChange);
  if (rows.length === 0) return null;

  return (
    <>
      <WorkspaceCredentialRows
        rows={rows}
        isWorkspaceOwner={isWorkspaceOwner}
        saving={actions.saving}
        actionError={actions.actionError && !actions.target}
        canGrant={actions.canGrant}
        canRevoke={actions.canRevoke}
        onRowRef={actions.setRowRef}
        onGrant={actions.openGrant}
        onRevoke={actions.revoke}
      />
      <WorkspaceCredentialConfirmation
        isMobile={isMobile}
        target={actions.target}
        saving={actions.saving}
        actionError={actions.actionError}
        onGrant={actions.grant}
        onCancel={actions.cancelGrant}
        onCloseAutoFocus={actions.restoreRowFocus}
        onAnimationEnd={actions.restoreRowFocusAfterDrawerClose}
      />
    </>
  );
}
