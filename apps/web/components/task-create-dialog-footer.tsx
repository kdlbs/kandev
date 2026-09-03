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
import { useTranslation } from "react-i18next";

type UpdateButtonProps = {
  isCreatingTask: boolean;
  hasTitle: boolean;
  onUpdate: () => void;
};

function UpdateButton({ isCreatingTask, hasTitle, onUpdate }: UpdateButtonProps) {
  const { t } = useTranslation();
  return (
    <Button
      type="button"
      variant="default"
      className="w-full h-11 cursor-pointer sm:w-auto sm:h-7 gap-1.5"
      disabled={isCreatingTask || !hasTitle}
      onClick={onUpdate}
    >
      {isCreatingTask ? (
        <>
          <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
          {t("task:updating2")}
        </>
      ) : (
        t("task:update")
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
  const { t } = useTranslation();
  const altLabel = isEditMode ? t("task:updateTask") : t("task:createOnly");

  return (
    <div className="flex flex-col w-full sm:w-auto gap-2 sm:gap-0">
      <div className="flex w-full sm:inline-flex sm:w-auto sm:h-7 h-11">
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
          {isCreatingTask ? t("task:starting") : t("task:startTask")}
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
                {t("task:startTaskInPlanMode")}
              </DropdownMenuItem>
            )}
            <DropdownMenuItem
              onClick={onAltAction}
              className="cursor-pointer whitespace-nowrap focus:bg-muted/80 hover:bg-muted/80"
              data-testid="submit-create-without-agent"
            >
              <IconPlus className="h-3.5 w-3.5 mr-1.5" />
              {isEditMode ? t("task:updateTask") : t("task:createWithoutStartingAgent")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {/* Mobile-only: visible buttons for plan mode and creating without agent */}
      {onPlanModeAction && (
        <Button
          type="button"
          variant="outline"
          className="w-full h-11 cursor-pointer gap-1.5 sm:hidden"
          disabled={altDisabled}
          onClick={onPlanModeAction}
          data-testid="mobile-plan-mode"
        >
          <IconFileInvoice className="h-3.5 w-3.5" />
          {t("task:planMode")}
        </Button>
      )}
      <Button
        type="button"
        variant="outline"
        className="w-full h-11 cursor-pointer gap-1.5 sm:hidden"
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
  disabled: boolean;
};

function DefaultSubmitButton({
  isCreatingSession,
  isCreatingTask,
  isSessionMode,
  isCreateMode,
  isEditMode,
  hasDescription,
  disabled,
}: DefaultSubmitButtonProps) {
  const { t } = useTranslation();
  const planModeStyle =
    isCreateMode && !hasDescription
      ? "bg-blue-600 border-blue-500 text-white hover:bg-blue-700 hover:text-white"
      : "";

  return (
    <Button
      type="submit"
      variant="default"
      className={`w-full h-11 cursor-pointer sm:w-auto sm:h-7 gap-1.5 ${planModeStyle}`}
      disabled={
        disabled || isCreatingSession || isCreatingTask || (isSessionMode ? !hasDescription : false)
      }
    >
      {(() => {
        if (isCreatingSession || isCreatingTask) {
          return (
            <>
              <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
              {isEditMode ? t("task:updating2") : t("task:starting")}
            </>
          );
        }
        if (isSessionMode) return t("task:createSession");
        if (isCreateMode) {
          return (
            <>
              <IconFileInvoice className="h-3.5 w-3.5" />
              {t("task:startPlanMode")}
            </>
          );
        }
        return t("task:updateTask");
      })()}
    </Button>
  );
}

export type TaskCreateDialogFooterProps = {
  isSessionMode: boolean;
  isCreateMode: boolean;
  isEditMode: boolean;
  autoTitle?: boolean;
  isTaskStarted: boolean;
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
    (!props.autoTitle && !props.hasTitle) ||
    (props.autoTitle && !props.hasDescription) ||
    !props.hasRepositorySelection ||
    !props.hasAllBranches ||
    missingCtx ||
    props.noCompatibleAgent
  );
}

export type ButtonKind = "update" | "start-task" | "default";

// Catalog keys, not copy. `computeDisabledReason` is a pure helper with no
// access to `t`, so it returns the key and the component resolves it at render
// (the repo-wide pattern for module-scope tables). Keeping these as keys also
// preserves the identity comparisons in the unit tests.
export const REASON_TITLE = "task:reasonAddTaskTitle";
export const REASON_PROMPT = "task:reasonAddTaskPrompt";
export const REASON_REPO = "task:reasonSelectRepository";
export const REASON_BRANCH = "task:reasonSelectBranch";
export const REASON_WORKSPACE = "task:reasonSelectWorkspace";
export const REASON_WORKFLOW = "task:reasonSelectWorkflow";
export const REASON_AGENT = "task:reasonSelectAgent";
export const REASON_DESCRIPTION = "task:reasonAddSessionDescription";
export const REASON_NO_COMPATIBLE_AGENT = "task:noCompatibleAgentProfileFor";

/**
 * Resolve what `computeDisabledReason` returned. Reasons this component owns are
 * catalog keys; `submitBlockedReason` is human text supplied by the caller and
 * passes through untouched.
 */
export function resolveDisabledReason(
  t: (key: string, options?: Record<string, unknown>) => string,
  reason: string | null | undefined,
  executorProfileName: string | null,
): string | undefined {
  if (!reason) return undefined;
  if (!reason.startsWith("task:")) return reason;
  return t(reason, {
    target: executorProfileName ? `“${executorProfileName}”` : t("task:thisExecutor"),
  });
}

function baseReason(props: TaskCreateDialogFooterProps): string | null {
  if (props.autoTitle && !props.hasDescription) return REASON_PROMPT;
  if (!props.autoTitle && !props.hasTitle) return REASON_TITLE;
  if (!props.hasRepositorySelection) return REASON_REPO;
  if (!props.hasAllBranches) return REASON_BRANCH;
  if (props.isCreateMode && !props.workspaceId) return REASON_WORKSPACE;
  if (props.isCreateMode && !props.effectiveWorkflowId) return REASON_WORKFLOW;
  if (props.noCompatibleAgent) return REASON_NO_COMPATIBLE_AGENT;
  return null;
}

function sessionDefaultReason(props: TaskCreateDialogFooterProps): string | null {
  if (props.noCompatibleAgent) return REASON_NO_COMPATIBLE_AGENT;
  if (!props.agentProfileId) return REASON_AGENT;
  if (!props.hasDescription) return REASON_DESCRIPTION;
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
    (props.isCreateMode && props.hasDescription) ||
    Boolean(props.isEditMode && props.agentProfileId);
  const blocked = Boolean(props.submitBlockedReason);
  const altDisabled = computeBaseDisabled(props) || blocked;
  const splitDisabled = altDisabled || !props.agentProfileId;
  // Session mode previously only gated on missing agent — it ignored
  // noCompatibleAgent, so a user who switched executor after picking an
  // agent could still submit a known-incompatible combination. The reason
  // text already surfaces REASON_NO_COMPATIBLE_AGENT in this branch (see
  // sessionDefaultReason), so the disable gate needs to match.
  const sessionDisabled = !props.agentProfileId || props.noCompatibleAgent;
  const defaultDisabled = (props.isSessionMode ? sessionDisabled : altDisabled) || blocked;

  const disabledReason = computeDisabledReason(props, resolveButtonKind(props, showStartTask));

  return { showStartTask, splitDisabled, altDisabled, defaultDisabled, disabledReason };
}

export function isNativeSubmitDisabled(props: TaskCreateDialogFooterProps): boolean {
  const { showStartTask, splitDisabled, defaultDisabled } = computeFooterState(props);
  if (props.isTaskStarted) return props.isCreatingTask || !props.hasTitle;
  return showStartTask ? splitDisabled : defaultDisabled;
}

export const TaskCreateDialogFooter = memo(function TaskCreateDialogFooter(
  props: TaskCreateDialogFooterProps,
) {
  const { t } = useTranslation();
  const {
    isSessionMode,
    isCreateMode,
    isEditMode,
    isTaskStarted,
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
          className="w-full h-11 border-0 cursor-pointer sm:w-auto sm:h-7 sm:border"
        >
          {t("common:cancel")}
        </Button>
      </DialogClose>
      <KeyboardShortcutTooltip
        shortcut={SHORTCUTS.SUBMIT}
        description={resolveDisabledReason(t, disabledReason, props.executorProfileName)}
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
                disabled={defaultDisabled}
              />
            );
          })()}
        </span>
      </KeyboardShortcutTooltip>
    </>
  );
});
