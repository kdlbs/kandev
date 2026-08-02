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

type SettingsFloatingSaveProps = {
  status: SettingsSaveStatus;
  dirtyContributorIds?: string;
  invalidReason?: string;
  navigationIntent: NavigationIntent | null;
  isDiscarding: boolean;
  onSave: () => Promise<boolean>;
  onDiscardAndLeave: () => Promise<void> | void;
  onContinueEditing: () => void;
};

export function SettingsFloatingSave({
  status,
  dirtyContributorIds,
  invalidReason,
  navigationIntent,
  isDiscarding,
  onSave,
  onDiscardAndLeave,
  onContinueEditing,
}: SettingsFloatingSaveProps) {
  const { t } = useTranslation();
  const isSaving = status === "saving";
  const isSaved = status === "saved";
  const isInvalid = Boolean(invalidReason);
  const isBusy = isSaving || isDiscarding;
  const { label: labelKey, accessible: accessibleKey } = saveButtonKeys(status);
  const accessibleLabel = t(accessibleKey);
  const configChatFloatingActionsHost = useConfigChatFloatingActionsHost();
  const isHostedByConfigChat = configChatFloatingActionsHost !== null;
  const saveAction = (
    <div
      className={cn(
        "pointer-events-none z-40 max-w-[calc(100vw_-_2rem_-_env(safe-area-inset-left)_-_env(safe-area-inset-right))]",
        !isHostedByConfigChat &&
          "fixed right-[calc(1rem_+_env(safe-area-inset-right))] bottom-[calc(5.25rem_+_env(safe-area-inset-bottom)_+_var(--app-status-bar-height))]",
      )}
      data-testid="settings-floating-save"
      data-dirty-contributors={dirtyContributorIds}
    >
      <div className="pointer-events-auto flex min-h-11 max-w-full flex-col items-stretch gap-2 rounded-md border bg-background p-2 shadow-lg sm:flex-row sm:items-center">
        {status === "error" && (
          <span className="flex items-center gap-1 text-xs text-destructive" role="status">
            <Trans i18nKey="settings:couldnTSave">
              <IconAlertCircle className="size-4" />
              {t("settings:couldnTSave2")}
            </Trans>
          </span>
        )}
        {invalidReason && (
          <span className="max-w-64 text-xs text-destructive" role="status">
            {invalidReason}
          </span>
        )}
        <Button
          type="button"
          size="lg"
          className="min-h-12 cursor-pointer bg-success text-success-foreground hover:bg-success/85 focus-visible:border-success focus-visible:ring-success/35"
          disabled={isBusy || isSaved || isInvalid}
          aria-label={accessibleLabel}
          onClick={() => void onSave()}
        >
          <SaveButtonIcon status={status} />
          {t(labelKey)}
        </Button>
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
function saveButtonKeys(status: SettingsSaveStatus): { label: string; accessible: string } {
  if (status === "saving") {
    return { label: "settings:saving", accessible: "settings:savingChanges" };
  }
  if (status === "saved") return { label: "settings:saved", accessible: "settings:saved" };
  if (status === "error") return { label: "settings:retrySave", accessible: "settings:retrySave" };
  return { label: "settings:saveChanges", accessible: "settings:saveChanges" };
}
