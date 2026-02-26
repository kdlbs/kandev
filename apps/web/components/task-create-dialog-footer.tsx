"use client";

import { memo } from "react";
import { IconLoader2, IconFileInvoice, IconSend, IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DialogClose } from "@kandev/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { KeyboardShortcutTooltip } from "@/components/keyboard-shortcut-tooltip";

type UpdateButtonProps = {
  isCreatingTask: boolean;
  hasTitle: boolean;
  onUpdate: () => void;
};

function UpdateButton({ isCreatingTask, hasTitle, onUpdate }: UpdateButtonProps) {
  return (
    <Button
      type="button"
      variant="default"
      className="w-full h-10 cursor-pointer sm:w-auto sm:h-7 gap-1.5"
      disabled={isCreatingTask || !hasTitle}
      onClick={onUpdate}
    >
      {isCreatingTask ? (
        <>
          <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
          Updating...
        </>
      ) : (
        "Update"
      )}
    </Button>
  );
}

type StartTaskSplitButtonProps = {
  isCreatingTask: boolean;
  disabled: boolean;
  isEditMode: boolean;
  onAltAction: () => void;
  onPlanModeAction?: () => void;
};

function StartTaskSplitButton({
  isCreatingTask,
  disabled,
  isEditMode,
  onAltAction,
  onPlanModeAction,
}: StartTaskSplitButtonProps) {
  return (
    <div className="inline-flex rounded-md border border-border overflow-hidden sm:h-7 h-10">
      <Button
        type="submit"
        variant="default"
        className="rounded-none border-0 cursor-pointer gap-1.5 h-full"
        disabled={disabled}
        data-testid="submit-start-agent"
      >
        {isCreatingTask ? (
          <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
        ) : (
          <IconSend className="h-3.5 w-3.5" />
        )}
        {isCreatingTask ? "Starting..." : "Start task"}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="default"
            className="rounded-none border-0 border-l border-primary-foreground/20 px-2 cursor-pointer h-full"
            disabled={disabled}
          >
            <IconChevronDown className="h-3.5 w-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-auto min-w-max">
          {onPlanModeAction && (
            <DropdownMenuItem
              onClick={onPlanModeAction}
              className="cursor-pointer whitespace-nowrap focus:bg-muted/80 hover:bg-muted/80"
              data-testid="submit-plan-mode"
            >
              <IconFileInvoice className="h-3.5 w-3.5 mr-1.5" />
              Start task in plan mode
            </DropdownMenuItem>
          )}
          <DropdownMenuItem
            onClick={onAltAction}
            className="cursor-pointer whitespace-nowrap focus:bg-muted/80 hover:bg-muted/80"
          >
            {isEditMode ? "Update task" : "Create without starting agent"}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

type DefaultSubmitButtonProps = {
  isCreatingSession: boolean;
  isCreatingTask: boolean;
  isSessionMode: boolean;
  isCreateMode: boolean;
  isEditMode: boolean;
  hasDescription: boolean;
  isPassthroughProfile: boolean;
  disabled: boolean;
};

function DefaultSubmitButton({
  isCreatingSession,
  isCreatingTask,
  isSessionMode,
  isCreateMode,
  isEditMode,
  hasDescription,
  isPassthroughProfile,
  disabled,
}: DefaultSubmitButtonProps) {
  const planModeStyle =
    isCreateMode && !hasDescription
      ? "bg-blue-600 border-blue-500 text-white hover:bg-blue-700 hover:text-white"
      : "";

  return (
    <Button
      type="submit"
      variant="default"
      className={`w-full h-10 cursor-pointer sm:w-auto sm:h-7 gap-1.5 ${planModeStyle}`}
      disabled={
        disabled ||
        isCreatingSession ||
        isCreatingTask ||
        (isSessionMode ? !hasDescription && !isPassthroughProfile : false)
      }
    >
      {(() => {
        if (isCreatingSession || isCreatingTask) {
          return (
            <>
              <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
              {isEditMode ? "Updating..." : "Starting..."}
            </>
          );
        }
        if (isSessionMode) return "Create Session";
        if (isCreateMode) {
          return (
            <>
              <IconFileInvoice className="h-3.5 w-3.5" />
              Start Plan Mode
            </>
          );
        }
        return "Update task";
      })()}
    </Button>
  );
}

export type TaskCreateDialogFooterProps = {
  isSessionMode: boolean;
  isCreateMode: boolean;
  isEditMode: boolean;
  isTaskStarted: boolean;
  isPassthroughProfile: boolean;
  isCreatingSession: boolean;
  isCreatingTask: boolean;
  hasTitle: boolean;
  hasDescription: boolean;
  hasRepositorySelection: boolean;
  branch: string;
  agentProfileId: string;
  workspaceId: string | null;
  effectiveWorkflowId: string | null;
  executorHint: string | null;
  onCancel: () => void;
  onUpdateWithoutAgent: () => void;
  onCreateWithoutAgent: () => void;
  onCreateWithPlanMode?: () => void;
};

function isMissingWorkflowCtx(
  isCreateMode: boolean,
  workspaceId: string | null,
  effectiveWorkflowId: string | null,
) {
  return isCreateMode && (!workspaceId || !effectiveWorkflowId);
}

function computeFooterState(props: TaskCreateDialogFooterProps) {
  const {
    isSessionMode,
    isCreateMode,
    isEditMode,
    isPassthroughProfile,
    isCreatingTask,
    hasTitle,
    hasDescription,
    hasRepositorySelection,
    branch,
    agentProfileId,
    workspaceId,
    effectiveWorkflowId,
  } = props;
  const showStartTask =
    (isCreateMode && (hasDescription || isPassthroughProfile)) ||
    Boolean(isEditMode && agentProfileId);
  const missingCtx = isMissingWorkflowCtx(isCreateMode, workspaceId, effectiveWorkflowId);
  const splitDisabled =
    isCreatingTask ||
    !hasTitle ||
    !hasRepositorySelection ||
    !branch ||
    !agentProfileId ||
    missingCtx;
  const defaultDisabled = isSessionMode
    ? !agentProfileId
    : !hasTitle || !hasRepositorySelection || !branch || missingCtx;
  return { showStartTask, splitDisabled, defaultDisabled };
}

export const TaskCreateDialogFooter = memo(function TaskCreateDialogFooter(
  props: TaskCreateDialogFooterProps,
) {
  const {
    isSessionMode,
    isCreateMode,
    isEditMode,
    isTaskStarted,
    isPassthroughProfile,
    isCreatingSession,
    isCreatingTask,
    hasTitle,
    hasDescription,
    executorHint,
    onCancel,
    onUpdateWithoutAgent,
    onCreateWithoutAgent,
    onCreateWithPlanMode,
  } = props;
  const { showStartTask, splitDisabled, defaultDisabled } = computeFooterState(props);

  return (
    <>
      {!isSessionMode && !isTaskStarted && executorHint && (
        <div className="flex flex-1 items-center gap-3 text-sm text-muted-foreground">
          <span className="text-xs text-muted-foreground">{executorHint}</span>
        </div>
      )}
      <DialogClose asChild>
        <Button
          type="button"
          variant="outline"
          onClick={onCancel}
          disabled={isCreatingSession || isCreatingTask}
          className="w-full h-10 border-0 cursor-pointer sm:w-auto sm:h-7 sm:border"
        >
          Cancel
        </Button>
      </DialogClose>
      <KeyboardShortcutTooltip shortcut={SHORTCUTS.SUBMIT}>
        {(() => {
          if (isTaskStarted) {
            return (
              <UpdateButton
                isCreatingTask={isCreatingTask}
                hasTitle={hasTitle}
                onUpdate={onUpdateWithoutAgent}
              />
            );
          }
          if (showStartTask) {
            return (
              <StartTaskSplitButton
                isCreatingTask={isCreatingTask}
                disabled={splitDisabled}
                isEditMode={isEditMode}
                onAltAction={isEditMode ? onUpdateWithoutAgent : onCreateWithoutAgent}
                onPlanModeAction={onCreateWithPlanMode}
              />
            );
          }
          return (
            <DefaultSubmitButton
              isCreatingSession={isCreatingSession}
              isCreatingTask={isCreatingTask}
              isSessionMode={isSessionMode}
              isCreateMode={isCreateMode}
              isEditMode={isEditMode}
              hasDescription={hasDescription}
              isPassthroughProfile={isPassthroughProfile}
              disabled={defaultDisabled}
            />
          );
        })()}
      </KeyboardShortcutTooltip>
    </>
  );
});
