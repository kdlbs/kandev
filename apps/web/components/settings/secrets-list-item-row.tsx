"use client";

import { useRef, useState, type RefObject } from "react";
import { Trans, useTranslation } from "react-i18next";
import { IconCopy, IconEdit, IconEye, IconEyeOff, IconKey, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { revealSecret } from "@/lib/api/domains/secrets-api";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { SecretDeleteConfirmation } from "./secrets-delete-dialog";

type SecretListItemRowProps = {
  secret: SecretListItem;
  workspaceId?: string;
  onEdit: (secret: SecretListItem) => void;
  onDelete: (secret: SecretListItem) => void;
  onDeleteCancel: () => void;
  onDeleteConfirm: () => void | Promise<void>;
  onCopyMove: (secret: SecretListItem) => void;
  isBusy: boolean;
  showCreate: boolean;
  isEditing: boolean;
  isDeleteConfirming: boolean;
};

type SecretListItemRowActionsProps = {
  secret: SecretListItem;
  onEdit: (secret: SecretListItem) => void;
  onDelete: (secret: SecretListItem) => void;
  onDeleteCancel: () => void;
  onDeleteConfirm: () => void | Promise<void>;
  onCopyMove: (secret: SecretListItem) => void;
  onReveal: () => void | Promise<void>;
  isBusy: boolean;
  showCreate: boolean;
  isEditing: boolean;
  isDeleteConfirming: boolean;
  isFinePointer: boolean;
  revealed: boolean;
  revealing: boolean;
  deleteAnchorRef: RefObject<HTMLButtonElement | null>;
};

type SecretListItemRowDeleteActionProps = {
  secret: SecretListItem;
  onDelete: (secret: SecretListItem) => void;
  onDeleteCancel: () => void;
  onDeleteConfirm: () => void | Promise<void>;
  isBusy: boolean;
  isDeleteConfirming: boolean;
  isFinePointer: boolean;
  deleteAnchorRef: RefObject<HTMLButtonElement | null>;
};

/** Renders a single secret row with local delete confirmation on every pointer type. */
export function SecretListItemRow({
  secret,
  workspaceId,
  onEdit,
  onDelete,
  onDeleteCancel,
  onDeleteConfirm,
  onCopyMove,
  isBusy,
  showCreate,
  isEditing,
  isDeleteConfirming,
}: SecretListItemRowProps) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const [revealed, setRevealed] = useState(false);
  const [revealedValue, setRevealedValue] = useState<string | null>(null);
  const [revealing, setRevealing] = useState(false);
  const deleteAnchorRef = useRef<HTMLButtonElement>(null);

  /** Fetches and shows the secret value, or hides it when already revealed. */
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
    <div
      className="rounded-lg border border-border/70 bg-background p-4 space-y-2"
      data-testid={`secret-row-${secret.id}`}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <IconKey className="h-4 w-4 text-muted-foreground shrink-0" />
          <div className="text-sm font-medium text-foreground truncate">{secret.name}</div>
          <Badge variant="outline" className="shrink-0 text-[10px]">
            {secret.scope === "workspace"
              ? t("settings:workspaceScope")
              : t("settings:globalScope")}
          </Badge>
        </div>
        <SecretListItemRowActions
          secret={secret}
          onEdit={onEdit}
          onDelete={onDelete}
          onDeleteCancel={onDeleteCancel}
          onDeleteConfirm={onDeleteConfirm}
          onCopyMove={onCopyMove}
          onReveal={handleReveal}
          isBusy={isBusy}
          showCreate={showCreate}
          isEditing={isEditing}
          isDeleteConfirming={isDeleteConfirming}
          isFinePointer={isFinePointer}
          revealed={revealed}
          revealing={revealing}
          deleteAnchorRef={deleteAnchorRef}
        />
      </div>
      {revealed && revealedValue !== null && (
        <div className="text-xs font-mono bg-muted/50 rounded px-2 py-1 break-all">
          {revealedValue}
        </div>
      )}
    </div>
  );
}

function SecretListItemRowActions({
  secret,
  onEdit,
  onDelete,
  onDeleteCancel,
  onDeleteConfirm,
  onCopyMove,
  onReveal,
  isBusy,
  showCreate,
  isEditing,
  isDeleteConfirming,
  isFinePointer,
  revealed,
  revealing,
  deleteAnchorRef,
}: SecretListItemRowActionsProps) {
  const { t } = useTranslation();
  return (
    <>
      <div
        className={`flex flex-wrap items-center justify-end gap-1 ${
          !isFinePointer && isDeleteConfirming ? "w-full min-w-0" : "shrink-0"
        }`}
      >
        {(!isDeleteConfirming || isFinePointer) && (
          <>
            <Button
              variant="ghost"
              onClick={() => onCopyMove(secret)}
              // A Move removes the source; it must never run while that secret's
              // edit/create draft is open (the draft would outlive its row).
              disabled={isBusy || showCreate || isEditing}
              className="min-h-11 cursor-pointer"
              aria-label={t("settings:copyMoveSecretNamed", { name: secret.name })}
            >
              <IconCopy className="h-4 w-4" />
              {t("settings:copyMove")}
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={onReveal}
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
          </>
        )}
        <SecretListItemRowDeleteAction
          secret={secret}
          onDelete={onDelete}
          onDeleteCancel={onDeleteCancel}
          onDeleteConfirm={onDeleteConfirm}
          isBusy={isBusy}
          isDeleteConfirming={isDeleteConfirming}
          isFinePointer={isFinePointer}
          deleteAnchorRef={deleteAnchorRef}
        />
      </div>
    </>
  );
}

function SecretListItemRowDeleteAction({
  secret,
  onDelete,
  onDeleteCancel,
  onDeleteConfirm,
  isBusy,
  isDeleteConfirming,
  isFinePointer,
  deleteAnchorRef,
}: SecretListItemRowDeleteActionProps) {
  const { t } = useTranslation();

  return (
    <>
      {!isFinePointer && isDeleteConfirming ? (
        <InlineConfirmActions
          density="touch"
          testId="secret-delete-inline-confirmation"
          ariaLabel={t("settings:deleteSecretNamed", { name: secret.name })}
          description={
            <Trans
              i18nKey="settings:thisWillPermanentlyRemoveSecret"
              values={{ name: secret.name }}
            >
              This will permanently remove{" "}
              <span className="font-medium text-foreground">{secret.name}</span>. This action cannot
              be undone.
            </Trans>
          }
          cancelLabel={t("settings:cancel")}
          confirmLabel={t("settings:deleteSecret")}
          confirmAriaLabel={t("settings:deleteSecretNamed", { name: secret.name })}
          confirmTestId="secret-delete-confirm"
          onCancel={onDeleteCancel}
          onClose={onDeleteCancel}
          onConfirm={onDeleteConfirm}
        />
      ) : (
        <Button
          ref={deleteAnchorRef}
          variant="ghost"
          size="icon"
          onClick={() => onDelete(secret)}
          disabled={isBusy}
          className="min-h-11 min-w-11 cursor-pointer"
          aria-label={t("settings:deleteSecretNamed", { name: secret.name })}
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      )}
      {isFinePointer ? (
        <SecretDeleteConfirmation
          secret={secret}
          open={isDeleteConfirming}
          anchorRef={deleteAnchorRef}
          onOpenChange={(open) => {
            if (!open) onDeleteCancel();
          }}
          onCancel={onDeleteCancel}
          onConfirm={onDeleteConfirm}
        />
      ) : null}
    </>
  );
}
