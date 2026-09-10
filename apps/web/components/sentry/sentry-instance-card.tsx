"use client";

import { useRef } from "react";
import { IconPencil, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  IntegrationAuthStatusBanner,
  type IntegrationAuthHealth,
} from "@/components/integrations/auth-status-banner";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { SentryConfig } from "@/lib/types/sentry";
import { useTranslation } from "react-i18next";

// configToHealth maps an instance's backend-recorded probe fields to the shared
// auth-status banner shape. Returns null (banner hidden) until a secret exists.
export function configToHealth(config: SentryConfig): IntegrationAuthHealth | null {
  if (!config.hasSecret) return null;
  if (!config.lastCheckedAt) return { ok: false, error: "", checkedAt: null };
  return {
    ok: !!config.lastOk,
    error: config.lastError ?? "",
    checkedAt: new Date(config.lastCheckedAt),
  };
}

type SentryInstanceCardProps = {
  instance: SentryConfig;
  onEdit: () => void;
  onDelete: () => void;
  isFinePointer: boolean;
  confirmingDelete: boolean;
  onDeleteCancel: (instanceId: string) => void;
  onDeleteConfirm: () => void;
};

// SentryInstanceCard renders one saved instance: its name, URL, per-instance
// auth-health banner, and edit/delete actions.
export function SentryInstanceCard({
  instance,
  onEdit,
  onDelete,
  isFinePointer,
  confirmingDelete,
  onDeleteCancel,
  onDeleteConfirm,
}: SentryInstanceCardProps) {
  const { t } = useTranslation();
  const deleteAnchorRef = useRef<HTMLButtonElement>(null);
  const confirmationTitle = t("sentry:removeInstanceConfirm", { name: instance.name });
  const { isMobile } = useResponsiveBreakpoint();
  const cancelDelete = () => onDeleteCancel(instance.id);
  const deleteLabel = t("sentry:delete");
  const actions = {
    cancelLabel: t("common:cancel"),
    confirmLabel: deleteLabel,
    confirmAriaLabel: confirmationTitle,
    confirmTestId: "sentry-remove-confirm",
    onConfirm: onDeleteConfirm,
  };
  return (
    <div className="space-y-3 rounded-md border p-4" data-testid="sentry-instance-card">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="font-medium truncate" data-testid="sentry-instance-name">
            {instance.name}
          </p>
          <p className="text-xs text-muted-foreground truncate">{instance.url}</p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={onEdit}
            aria-label={t("sentry:editInstanceAria", { name: instance.name })}
            className="cursor-pointer gap-1"
            data-testid="sentry-instance-edit-button"
          >
            <IconPencil className="h-3.5 w-3.5" />
            {t("sentry:edit")}
          </Button>
          {(isMobile || isFinePointer || !confirmingDelete) && (
            <Button
              ref={deleteAnchorRef}
              type="button"
              size="sm"
              variant="destructive"
              onClick={onDelete}
              aria-label={t("sentry:deleteInstanceAria", { name: instance.name })}
              className="min-h-11 cursor-pointer gap-1"
              data-testid="sentry-instance-delete-button"
            >
              <IconTrash className="h-3.5 w-3.5" />
              {deleteLabel}
            </Button>
          )}
        </div>
      </div>
      <MobileActionConfirmation
        open={confirmingDelete}
        targetKey={`${instance.workspaceId}:${instance.id}`}
        title={confirmationTitle}
        {...actions}
        onOpenChange={(open) => {
          if (!open) cancelDelete();
        }}
        focusReturnRef={deleteAnchorRef}
        fallback={
          !isFinePointer ? (
            <InlineConfirmActions
              density="touch"
              testId="sentry-remove-inline-confirmation"
              ariaLabel={confirmationTitle}
              description={confirmationTitle}
              {...actions}
              onCancel={cancelDelete}
              onClose={cancelDelete}
            />
          ) : (
            <ActionConfirmPopover
              open={confirmingDelete}
              anchorRef={deleteAnchorRef}
              title={confirmationTitle}
              {...actions}
              testId="sentry-remove-confirm-popover"
              onOpenChange={(open) => {
                if (!open) cancelDelete();
              }}
              onCancel={cancelDelete}
            />
          )
        }
      />
      <IntegrationAuthStatusBanner health={configToHealth(instance)} />
    </div>
  );
}
