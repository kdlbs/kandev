"use client";

import { useId, type RefObject } from "react";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import {
  ChangeWorkflowActionFooter,
  ChangeWorkflowAgentSection,
  ChangeWorkflowCurrentTask,
  ChangeWorkflowEntryPreview,
  ChangeWorkflowSelectors,
  ChangeWorkflowSubmitError,
} from "@/components/task/change-workflow-form-sections";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { createFocusReturnHandler } from "@/lib/dialog-focus-return";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { useChangeWorkflow } from "@/hooks/domains/kanban/use-change-workflow";

type ChangeWorkflowDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string | null;
  workspaceId: string | null;
  focusReturnRef?: RefObject<HTMLElement | null>;
  onSuccess?: () => void;
};

function ChangeWorkflowForm({
  taskId,
  workspaceId,
  open,
  onOpenChange,
  onSuccess,
  isTouchSurface,
}: {
  taskId: string | null;
  workspaceId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
  isTouchSurface: boolean;
}) {
  const workflowSelectId = useId();
  const stepSelectId = useId();
  const state = useChangeWorkflow({ open, taskId, workspaceId, onOpenChange, onSuccess });
  const sectionClass = cn("grid gap-3", isTouchSurface && "gap-4");

  return (
    <form
      className="flex min-h-0 flex-1 flex-col"
      data-testid="change-workflow-form"
      onSubmit={(event) => {
        event.preventDefault();
        void state.submit();
      }}
    >
      <div
        data-testid="change-workflow-scroll"
        className={cn(
          "min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-4",
          isTouchSurface && "px-4 py-3",
        )}
      >
        <div className={sectionClass}>
          <ChangeWorkflowCurrentTask state={state} />
          <ChangeWorkflowSelectors
            state={state}
            isTouchSurface={isTouchSurface}
            workflowSelectId={workflowSelectId}
            stepSelectId={stepSelectId}
          />
          <ChangeWorkflowAgentSection state={state} isTouchSurface={isTouchSurface} />
          <ChangeWorkflowEntryPreview state={state} isTouchSurface={isTouchSurface} />
          <ChangeWorkflowSubmitError state={state} />
        </div>
      </div>
      <ChangeWorkflowActionFooter
        state={state}
        isTouchSurface={isTouchSurface}
        onOpenChange={onOpenChange}
      />
    </form>
  );
}

export function ChangeWorkflowDialog({
  open,
  onOpenChange,
  taskId,
  workspaceId,
  focusReturnRef,
  onSuccess,
}: ChangeWorkflowDialogProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const closeFocus = createFocusReturnHandler(focusReturnRef);
  const surfaceContent = (
    <>
      <div className="flex shrink-0 items-start justify-between gap-3 border-b border-border/70 px-5 py-4">
        <div className="min-w-0">
          {isMobile ? (
            <DrawerHeader className="p-0 text-left">
              <DrawerTitle className="text-base">{t("task:changeWorkflowTitle")}</DrawerTitle>
              <DrawerDescription>{t("task:changeWorkflowDescription")}</DrawerDescription>
            </DrawerHeader>
          ) : (
            <DialogHeader className="text-left">
              <DialogTitle className="text-base">{t("task:changeWorkflowTitle")}</DialogTitle>
              <DialogDescription>{t("task:changeWorkflowDescription")}</DialogDescription>
            </DialogHeader>
          )}
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={cn("shrink-0", isMobile && "size-11")}
          aria-label={t("common:close")}
          onClick={() => onOpenChange(false)}
          data-testid="change-workflow-close"
        >
          <IconX className="size-4" aria-hidden="true" />
        </Button>
      </div>
      <ChangeWorkflowForm
        taskId={taskId}
        workspaceId={workspaceId}
        open={open}
        onOpenChange={onOpenChange}
        onSuccess={onSuccess}
        isTouchSurface={isMobile}
      />
    </>
  );

  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          className="!top-0 !bottom-auto !mt-0 !h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] !max-h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] min-h-0 overflow-hidden p-0 outline-none"
          data-testid="change-workflow-drawer"
          onCloseAutoFocus={closeFocus}
        >
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl bg-background">
            {surfaceContent}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="flex max-h-[min(90dvh,900px)] min-h-0 flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl"
        data-testid="change-workflow-dialog"
        showCloseButton={false}
        onCloseAutoFocus={closeFocus}
      >
        {surfaceContent}
      </DialogContent>
    </Dialog>
  );
}
