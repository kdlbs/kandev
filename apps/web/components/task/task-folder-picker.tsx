"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { useTaskFolderAction } from "@/hooks/use-task-folder-action";

export function TaskFolderPicker({ action }: { action: ReturnType<typeof useTaskFolderAction> }) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touch = isMobile || !isFinePointer;
  const choices = (
    <div className="flex flex-col gap-1">
      {action.options.map((option) => (
        <Button
          key={option.worktreeId}
          variant="ghost"
          className={`w-full justify-start cursor-pointer ${touch ? "h-11 min-h-11" : "h-7"}`}
          onClick={() => action.select(option.worktreeId)}
          disabled={action.isLoading}
        >
          <span className="min-w-0 truncate">{option.label}</span>
          {option.branch && (
            <span className="ml-auto min-w-0 truncate text-muted-foreground">{option.branch}</span>
          )}
        </Button>
      ))}
    </div>
  );
  const title = t("editors:chooseFolder");
  const description = t("editors:openFolderOnHost");
  if (touch) {
    return (
      <MobilePickerSheet
        open={action.pickerOpen}
        onOpenChange={action.onOpenChange}
        onCloseAutoFocus={action.onCloseAutoFocus}
        title={title}
        description={description}
        contentTestId="task-folder-picker"
      >
        {choices}
      </MobilePickerSheet>
    );
  }
  return (
    <Dialog open={action.pickerOpen} onOpenChange={action.onOpenChange}>
      <DialogContent
        showCloseButton={false}
        enterConfirms={false}
        onCloseAutoFocus={action.onCloseAutoFocus}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="max-h-[60dvh] overflow-y-auto" data-testid="task-folder-picker">
          {choices}
        </div>
        <Button
          variant="outline"
          className="justify-self-end cursor-pointer"
          onClick={() => action.onOpenChange(false)}
        >
          {t("common:cancel")}
        </Button>
      </DialogContent>
    </Dialog>
  );
}
