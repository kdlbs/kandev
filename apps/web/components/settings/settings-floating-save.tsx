"use client";
import { Trans, useTranslation } from "react-i18next";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Button } from "@kandev/ui/button";
import { IconAlertCircle, IconCheck, IconDeviceFloppy, IconLoader2 } from "@tabler/icons-react";
import { createPortal } from "react-dom";

import { useConfigChatFloatingActionsHost } from "@/components/config-chat/config-chat-provider";
import type { NavigationIntent } from "@/lib/routing/navigation-guard";
import { cn } from "@/lib/utils";

export type SettingsSaveStatus = "dirty" | "saving" | "saved" | "error";
export type SettingsSaveErrorKind = "save" | "reset";
export type SettingsSavePlacement = "content" | "viewport";

function standalonePositioningClass(placement: SettingsSavePlacement): string {
  if (placement === "content") {
    return "absolute inset-x-0 bottom-[calc(5rem_+_env(safe-area-inset-bottom))] pl-[calc(1rem_+_env(safe-area-inset-left))] pr-[calc(1rem_+_env(safe-area-inset-right))] md:bottom-[calc(1.6875rem_+_env(safe-area-inset-bottom))]";
  }
  return "fixed inset-x-0 bottom-[calc(5.25rem_+_env(safe-area-inset-bottom)_+_var(--app-status-bar-height))] pl-[calc(1rem_+_env(safe-area-inset-left))] pr-[calc(1rem_+_env(safe-area-inset-right))]";
}

type SettingsFloatingSaveProps = {
  status: SettingsSaveStatus;
  placement?: SettingsSavePlacement;
  canSave?: boolean;
  errorKind?: SettingsSaveErrorKind | null;
  dirtyContributorIds?: string;
  invalidReason?: string;
  navigationIntent: NavigationIntent | null;
  isDiscarding: boolean;
  onSave: () => Promise<boolean>;
  onReset: () => Promise<unknown> | void;
  onDiscardAndLeave: () => Promise<void> | void;
  onContinueEditing: () => void;
};

export function SettingsFloatingSave({
  status,
  placement = "viewport",
  canSave = true,
  errorKind,
  dirtyContributorIds,
  invalidReason,
  navigationIntent,
  isDiscarding,
  onSave,
  onReset,
  onDiscardAndLeave,
  onContinueEditing,
}: SettingsFloatingSaveProps) {
  const { t } = useTranslation();
  const isSaving = status === "saving";
  const isSaved = status === "saved";
  const isInvalid = !canSave || Boolean(invalidReason);
  const isBusy = isSaving || isDiscarding;
  const { label: labelKey, accessible: accessibleKey } = saveButtonKeys(status, errorKind);
  const errorMessage = errorMessageKeys(errorKind);
  const accessibleLabel = t(accessibleKey);
  const configChatFloatingActionsHost = useConfigChatFloatingActionsHost();
  const isHostedByConfigChat = configChatFloatingActionsHost !== null;
  const positioningClass = standalonePositioningClass(placement);
  const primaryAction = status === "error" && errorKind === "reset" ? onReset : onSave;
  const saveAction = (
    <div
      className={cn(
        "pointer-events-none z-40 flex w-full justify-center",
        !isHostedByConfigChat && positioningClass,
      )}
      data-testid="settings-floating-save"
      data-dirty-contributors={dirtyContributorIds}
      data-status={status}
    >
      <div
        className="pointer-events-auto flex w-fit max-w-full items-center gap-1 rounded-lg border border-border/80 bg-card/95 px-1 shadow-md backdrop-blur-sm md:py-0.5"
        data-testid="settings-floating-save-surface"
      >
        <div className="min-w-0 max-w-52 flex-1 space-y-0.5 px-1">
          {status === "error" ? (
            <span className="flex items-center gap-1 text-xs text-destructive" role="status">
              <Trans i18nKey={errorMessage.labelKey}>
                <IconAlertCircle className="size-4" />
                {t(errorMessage.fallbackKey)}
              </Trans>
            </span>
          ) : (
            <span className="text-sm font-medium text-foreground">
              {isSaved ? t("settings:saved") : t("common:unsavedChanges")}
            </span>
          )}
          {invalidReason && (
            <span className="block max-w-64 text-xs text-destructive" role="status">
              {invalidReason}
            </span>
          )}
        </div>
        <div className="flex shrink-0 gap-2">
          <Button
            type="button"
            variant="outline"
            className="h-11 min-h-11 shrink-0 cursor-pointer px-3 text-sm md:h-8 md:min-h-8"
            disabled={isBusy || isSaved}
            onClick={() => void onReset()}
          >
            {t("settings:reset")}
          </Button>
          <Button
            type="button"
            size="default"
            className="h-11 min-h-11 shrink-0 cursor-pointer bg-success px-3 text-sm text-success-foreground hover:bg-success/85 focus-visible:border-success focus-visible:ring-success/35 md:h-8 md:min-h-8"
            disabled={isBusy || isSaved || isInvalid}
            aria-label={accessibleLabel}
            onClick={() => void primaryAction()}
          >
            <SaveButtonIcon status={status} />
            {t(labelKey)}
          </Button>
        </div>
      </div>
    </div>
  );

  return (
    <>
      {isHostedByConfigChat ? createPortal(saveAction, configChatFloatingActionsHost) : saveAction}

      <LeaveWithUnsavedChangesDialog
        open={navigationIntent !== null}
        isBusy={isBusy}
        isInvalid={isInvalid}
        isDiscarding={isDiscarding}
        isSaving={isSaving}
        onSave={onSave}
        onDiscardAndLeave={onDiscardAndLeave}
        onContinueEditing={onContinueEditing}
      />
    </>
  );
}

// LeaveWithUnsavedChangesDialog is the navigation-guard prompt offering save,
// discard, or continue editing when leaving a page with unsaved settings.
function LeaveWithUnsavedChangesDialog({
  open,
  isBusy,
  isInvalid,
  isDiscarding,
  isSaving,
  onSave,
  onDiscardAndLeave,
  onContinueEditing,
}: {
  open: boolean;
  isBusy: boolean;
  isInvalid: boolean;
  isDiscarding: boolean;
  isSaving: boolean;
  onSave: () => Promise<boolean>;
  onDiscardAndLeave: () => Promise<void> | void;
  onContinueEditing: () => void;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={open}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("settings:saveChangesBeforeLeaving")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("settings:thisPageHasUnsavedChangesSave")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel
            className="cursor-pointer"
            disabled={isBusy}
            onClick={onContinueEditing}
          >
            {t("settings:continueEditing")}
          </AlertDialogCancel>
          <Button
            type="button"
            variant="outline"
            className="cursor-pointer"
            disabled={isBusy}
            onClick={() => void onDiscardAndLeave()}
          >
            {isDiscarding ? t("settings:discarding") : t("settings:discardAndLeave")}
          </Button>
          <AlertDialogAction
            className="cursor-pointer bg-success text-success-foreground hover:bg-success/85 focus-visible:border-success focus-visible:ring-success/35"
            data-dialog-default-action
            disabled={isBusy || isInvalid}
            onClick={(event) => {
              event.preventDefault();
              void onSave();
            }}
          >
            {isSaving ? t("settings:saving") : t("settings:saveAndLeave")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function SaveButtonIcon({ status }: { status: SettingsSaveStatus }) {
  if (status === "saving") return <IconLoader2 className="size-4 animate-spin" />;
  if (status === "saved") return <IconCheck className="size-4" />;
  return <IconDeviceFloppy className="size-4" />;
}

/**
 * Catalog keys for the save button, by status.
 *
 * Returns KEYS, not resolved copy: the caller resolves them with `t()` at render
 * so a locale switch re-renders. The visible label is deliberately shorter than
 * the accessible name while saving ("Saving…" vs "Saving changes"), so the two
 * are separate keys rather than one string compared against itself.
 */
function saveButtonKeys(
  status: SettingsSaveStatus,
  errorKind?: SettingsSaveErrorKind | null,
): { label: string; accessible: string } {
  if (status === "saving") {
    return { label: "settings:saving", accessible: "settings:savingChanges" };
  }
  if (status === "saved") return { label: "settings:saved", accessible: "settings:saved" };
  if (status === "error" && errorKind === "reset") {
    return { label: "settings:retryReset", accessible: "settings:retryReset" };
  }
  if (status === "error") return { label: "settings:retrySave", accessible: "settings:retrySave" };
  return { label: "settings:saveChanges", accessible: "settings:saveChanges" };
}

function errorMessageKeys(errorKind: SettingsSaveErrorKind | null | undefined): {
  labelKey: string;
  fallbackKey: string;
} {
  if (errorKind === "reset") {
    return { labelKey: "settings:couldnTReset", fallbackKey: "settings:couldnTReset2" };
  }
  return { labelKey: "settings:couldnTSave", fallbackKey: "settings:couldnTSave2" };
}
