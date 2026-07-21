"use client";

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
  const isSaving = status === "saving";
  const isSaved = status === "saved";
  const isInvalid = Boolean(invalidReason);
  const isBusy = isSaving || isDiscarding;
  const buttonLabel = status === "error" ? "Retry save" : "Save changes";
  const accessibleLabel = getAccessibleLabel(status, buttonLabel);
  const configChatFloatingActionsHost = useConfigChatFloatingActionsHost();
  const isHostedByConfigChat = configChatFloatingActionsHost !== null;
  const saveAction = (
    <div
      className={cn(
        "pointer-events-none z-40 max-w-[calc(100vw_-_2rem_-_env(safe-area-inset-left)_-_env(safe-area-inset-right))]",
        !isHostedByConfigChat &&
          "fixed right-[calc(1rem_+_env(safe-area-inset-right))] bottom-[calc(5.25rem_+_env(safe-area-inset-bottom))]",
      )}
      data-testid="settings-floating-save"
      data-dirty-contributors={dirtyContributorIds}
    >
      <div className="pointer-events-auto flex min-h-11 max-w-full flex-col items-stretch gap-2 rounded-md border bg-background p-2 shadow-lg sm:flex-row sm:items-center">
        {status === "error" && (
          <span className="flex items-center gap-1 text-xs text-destructive" role="status">
            <IconAlertCircle className="size-4" />
            Couldn't save
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
          {accessibleLabel === "Saving changes" ? "Saving..." : accessibleLabel}
        </Button>
      </div>
    </div>
  );

  return (
    <>
      {isHostedByConfigChat ? createPortal(saveAction, configChatFloatingActionsHost) : saveAction}

      <AlertDialog open={navigationIntent !== null}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Save changes before leaving?</AlertDialogTitle>
            <AlertDialogDescription>
              This page has unsaved changes. Save them, discard them, or continue editing.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel
              className="cursor-pointer"
              disabled={isBusy}
              onClick={onContinueEditing}
            >
              Continue editing
            </AlertDialogCancel>
            <Button
              type="button"
              variant="outline"
              className="cursor-pointer"
              disabled={isBusy}
              onClick={() => void onDiscardAndLeave()}
            >
              {isDiscarding ? "Discarding..." : "Discard and leave"}
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
              {isSaving ? "Saving..." : "Save and leave"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

function SaveButtonIcon({ status }: { status: SettingsSaveStatus }) {
  if (status === "saving") return <IconLoader2 className="size-4 animate-spin" />;
  if (status === "saved") return <IconCheck className="size-4" />;
  return <IconDeviceFloppy className="size-4" />;
}

function getAccessibleLabel(status: SettingsSaveStatus, buttonLabel: string): string {
  if (status === "saving") return "Saving changes";
  if (status === "saved") return "Saved";
  return buttonLabel;
}
