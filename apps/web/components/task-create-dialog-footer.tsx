"use client";

import { memo } from "react";
import {
  IconLoader2,
  IconFileInvoice,
  IconSend,
  IconChevronDown,
  IconPlus,
} from "@tabler/icons-react";
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
  altDisabled: boolean;
  isEditMode: boolean;
  onAltAction: () => void;
  onPlanModeAction?: () => void;
};

function StartTaskSplitButton({
  isCreatingTask,
  disabled,
  altDisabled,
  isEditMode,
  onAltAction,
  onPlanModeAction,
}: StartTaskSplitButtonProps) {
  const altLabel = isEditMode ? "Update task" : "Create only";

  return (
    <div className="flex flex-col w-full sm:w-auto gap-2 sm:gap-0">
      <div className="flex w-full sm:inline-flex sm:w-auto sm:h-7 h-10">
        <Button
          type="submit"
          variant="default"
          className="h-full flex-1 cursor-pointer gap-1.5 sm:rounded-r-none sm:border-r-0"
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
              className="-ml-px h-full hidden rounded-l-none border-l border-primary-foreground/20 px-2 cursor-pointer sm:flex"
              disabled={disabled}
              data-testid="submit-start-agent-chevron"
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
              data-testid="submit-create-without-agent"
            >
              <IconPlus className="h-3.5 w-3.5 mr-1.5" />
              {isEditMode ? "Update task" : "Create without starting agent"}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {/* Mobile-only: visible buttons for plan mode and creating without agent */}
      {onPlanModeAction && (
        <Button
          type="button"
          variant="outline"
          className="w-full h-10 cursor-pointer gap-1.5 sm:hidden"
          disabled={altDisabled}
          onClick={onPlanModeAction}
          data-testid="mobile-plan-mode"
        >
          <IconFileInvoice className="h-3.5 w-3.5" />
          Plan mode
        </Button>
      )}
      <Button
        type="button"
        variant="outline"
        className="w-full h-10 cursor-pointer gap-1.5 sm:hidden"
        disabled={altDisabled}
        onClick={onAltAction}
      >
        <IconPlus className="h-3.5 w-3.5" />
        {altLabel}
      </Button>
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
  /**
   * True when every selected repo has a base branch picked (and the URL flow's
   * branch, if active). Was previously `branch: string` for the single-repo
   * primary; now aggregated upstream because each row has its own branch.
   */
  hasAllBranches: boolean;
  agentProfileId: string;
  workspaceId: string | null;
  effectiveWorkflowId: string | null;
  executorHint: string | null;
  noCompatibleAgent: boolean;
  executorProfileName: string | null;
  onCancel: () => void;
  onUpdateWithoutAgent: () => void;
  onCreateWithoutAgent: () => void;
  onCreateWithPlanMode?: () => void;
  /**
   * Externally-supplied reason that the submit buttons are disabled (e.g. an
   * async bootstrap step from a feature wrapper hasn't finished yet). When set,
   * every submit variant is disabled and the tooltip shows this string instead
   * of the usual missing-field reason.
   */
  submitBlockedReason?: string | null;
};

function isMissingWorkflowCtx(
  isCreateMode: boolean,
  workspaceId: string | null,
  effectiveWorkflowId: string | null,
) {
  return isCreateMode && (!workspaceId || !effectiveWorkflowId);
}

function computeBaseDisabled(props: TaskCreateDialogFooterProps) {
  const missingCtx = isMissingWorkflowCtx(
    props.isCreateMode,
    props.workspaceId,
    props.effectiveWorkflowId,
  );
  return (
    props.isCreatingTask ||
    !props.hasTitle ||
    !props.hasRepositorySelection ||
    !props.hasAllBranches ||
    missingCtx ||
    props.noCompatibleAgent
  );
}

export type ButtonKind = "update" | "start-task" | "default";

export const REASON_TITLE = "Add a task title";
export const REASON_REPO = "Select a repository";
export const REASON_BRANCH = "Select a branch";
export const REASON_WORKSPACE = "Select a workspace";
export const REASON_WORKFLOW = "Select a workflow";
export const REASON_AGENT = "Select an agent";
export const REASON_DESCRIPTION = "Add a session description";

function noCompatibleAgentReason(executorProfileName: string | null): string {
  const target = executorProfileName ? `“${executorProfileName}”` : "this executor";
  return `No compatible agent profile is configured for ${target}. Configure agent credentials in Settings → Executors.`;
}

function baseReason(props: TaskCreateDialogFooterProps): string | null {
  if (!props.hasTitle) return REASON_TITLE;
  if (!props.hasRepositorySelection) return REASON_REPO;
  if (!props.hasAllBranches) return REASON_BRANCH;
  if (props.isCreateMode && !props.workspaceId) return REASON_WORKSPACE;
  if (props.isCreateMode && !props.effectiveWorkflowId) return REASON_WORKFLOW;
  if (props.noCompatibleAgent) return noCompatibleAgentReason(props.executorProfileName);
  return null;
}

function missingSessionDescription(props: TaskCreateDialogFooterProps): boolean {
  return !props.hasDescription && !props.isPassthroughProfile;
}

function sessionDefaultReason(props: TaskCreateDialogFooterProps): string | null {
  if (props.noCompatibleAgent) return noCompatibleAgentReason(props.executorProfileName);
  if (!props.agentProfileId) return REASON_AGENT;
  if (missingSessionDescription(props)) return REASON_DESCRIPTION;
  return null;
}

export function computeDisabledReason(
  props: TaskCreateDialogFooterProps,
  kind: ButtonKind,
): string | null {
  if (props.isCreatingTask) return null;
  if (props.submitBlockedReason) return props.submitBlockedReason;
  if (kind === "update") return props.hasTitle ? null : REASON_TITLE;
  if (kind === "default" && props.isSessionMode) return sessionDefaultReason(props);
  const base = baseReason(props);
  if (base) return base;
  if (kind === "start-task" && !props.agentProfileId) return REASON_AGENT;
  return null;
}

function resolveButtonKind(props: TaskCreateDialogFooterProps, showStartTask: boolean): ButtonKind {
  if (props.isTaskStarted) return "update";
  if (showStartTask) return "start-task";
  return "default";
}

function computeFooterState(props: TaskCreateDialogFooterProps) {
  const showStartTask =
    (props.isCreateMode && (props.hasDescription || props.isPassthroughProfile)) ||
    Boolean(props.isEditMode && props.agentProfileId);
  const blocked = Boolean(props.submitBlockedReason);
  const altDisabled = computeBaseDisabled(props) || blocked;
  const splitDisabled = altDisabled || !props.agentProfileId;
  // Session mode previously only gated on missing agent — it ignored
  // noCompatibleAgent, so a user who switched executor after picking an
  // agent could still submit a known-incompatible combination. The reason
  // text already surfaces noCompatibleAgentReason in this branch (see
  // sessionDefaultReason), so the disable gate needs to match.
  const sessionDisabled = !props.agentProfileId || props.noCompatibleAgent;
  const defaultDisabled = (props.isSessionMode ? sessionDisabled : altDisabled) || blocked;

  const disabledReason = computeDisabledReason(props, resolveButtonKind(props, showStartTask));

  return { showStartTask, splitDisabled, altDisabled, defaultDisabled, disabledReason };
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
  const { showStartTask, splitDisabled, altDisabled, defaultDisabled, disabledReason } =
    computeFooterState(props);

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
      <KeyboardShortcutTooltip
        shortcut={SHORTCUTS.SUBMIT}
        description={disabledReason ?? undefined}
      >
        <span className="inline-flex w-full sm:w-auto" data-testid="submit-start-agent-wrapper">
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
                  altDisabled={altDisabled}
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
        </span>
      </KeyboardShortcutTooltip>
    </>
  );
});
