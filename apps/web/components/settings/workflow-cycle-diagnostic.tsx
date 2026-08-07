"use client";

import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconArrowRight, IconUser } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
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
import type {
  WorkflowReplayCycleDiagnostic,
  WorkflowReplayCycleHop,
} from "@/lib/workflows/replay-cycle-analysis";
import type { WorkflowMutationProposal } from "./workflow-mutation-guard";
import { cn } from "@/lib/utils";

// `promptSource`, `trigger` and `actionKind` are wire values from the analyzer.
// Each maps to a catalog key resolved at render; the step name it describes is
// user data and travels as an interpolated value, never inside the message.
const PROMPT_SOURCE_KEYS = {
  task_description: "workflows:promptSourceTaskDescription",
  step_prompt_with_task_description: "workflows:promptSourceStepPromptWithTaskDescription",
  step_prompt: "workflows:promptSourceStepPrompt",
} as const;

const TRIGGER_LABEL_KEYS = {
  on_turn_start: "workflows:onTurnStartTrigger",
  on_turn_complete: "workflows:onTurnCompleteTrigger",
} as const;

const ACTION_LABEL_KEYS = {
  move_to_next: "workflows:moveToNextStep",
  move_to_previous: "workflows:moveToPreviousStep",
  move_to_step: "workflows:moveToSpecificStep",
} as const;

function CycleHop({ hop, index }: { hop: WorkflowReplayCycleHop; index: number }) {
  const { t } = useTranslation();
  return (
    <li className="min-w-0 rounded-md border bg-background/60 p-3">
      <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-2">
        <span className="min-w-0 break-words font-medium">{hop.sourceStepName}</span>
        <IconArrowRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="min-w-0 break-words text-right font-medium">
          {hop.destinationStepName}
        </span>
      </div>
      <div className="mt-2 flex min-w-0 flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
        <span className="whitespace-nowrap rounded bg-muted px-2 py-1">
          {t(TRIGGER_LABEL_KEYS[hop.trigger])}
        </span>
        <span className="whitespace-nowrap rounded bg-muted px-2 py-1">
          {t(ACTION_LABEL_KEYS[hop.actionKind])}
        </span>
      </div>
      {hop.requiresUserInvolvement && (
        <div className="mt-2 flex items-center gap-1.5 text-xs font-medium text-foreground">
          <IconUser className="size-3.5 shrink-0" aria-hidden="true" />
          <span>{t("workflows:userActionRequired")}</span>
        </div>
      )}
      <span className="sr-only">{t("workflows:hopNumber", { index: index + 1 })}</span>
    </li>
  );
}

export function WorkflowCycleDiagnostic({
  diagnostic,
}: {
  diagnostic: WorkflowReplayCycleDiagnostic;
}) {
  const { t } = useTranslation();
  const isBlocking = diagnostic.severity === "blocking";
  const promptText = t(PROMPT_SOURCE_KEYS[diagnostic.promptSource], {
    stepName: diagnostic.autoStartStepName,
  });

  return (
    <Alert
      variant={isBlocking ? "destructive" : "default"}
      className={cn(
        "min-w-0 overflow-hidden p-3 text-sm",
        !isBlocking && "border-amber-500/60 bg-amber-500/5",
      )}
      data-testid={`workflow-cycle-diagnostic-${diagnostic.autoStartStepId}`}
    >
      <IconAlertTriangle className="mt-0.5 size-4" aria-hidden="true" />
      <AlertTitle className="text-sm">
        {isBlocking
          ? t("workflows:automaticWorkflowCycle")
          : t("workflows:potentialRepeatedAgentRun")}
      </AlertTitle>
      <AlertDescription className="min-w-0 space-y-3 text-left text-sm text-pretty">
        <p>
          {isBlocking
            ? t("workflows:workflowCycleBlockingBody", {
                stepName: diagnostic.autoStartStepName,
              })
            : t("workflows:workflowCycleWarningBody", {
                stepName: diagnostic.autoStartStepName,
              })}
        </p>
        <ol
          aria-label={t("workflows:workflowCycleReplayPathFor", {
            stepName: diagnostic.autoStartStepName,
          })}
          className="grid min-w-0 gap-2"
        >
          {diagnostic.trace.map((hop, index) => (
            <CycleHop
              key={`${diagnostic.identity}-${hop.sourceStepId}-${hop.trigger}-${index}`}
              hop={hop}
              index={index}
            />
          ))}
        </ol>
        <p className="break-words">{promptText}</p>
      </AlertDescription>
    </Alert>
  );
}

type WorkflowCycleGuardDialogProps = {
  proposal: WorkflowMutationProposal | null;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
};

export function WorkflowCycleGuardDialog({
  proposal,
  onCancel,
  onConfirm,
}: WorkflowCycleGuardDialogProps) {
  const { t } = useTranslation();
  const isBlocking = proposal?.severity === "blocking";
  const actionLabel =
    proposal?.intent === "create" ? t("workflows:createAnyway") : t("workflows:applyAnyway");
  const confirming = useRef(false);

  const handleConfirm = async () => {
    confirming.current = true;
    try {
      await onConfirm();
    } finally {
      confirming.current = false;
    }
  };

  return (
    <AlertDialog
      open={proposal !== null}
      onOpenChange={(open) => !open && !confirming.current && onCancel()}
    >
      <AlertDialogContent
        className="max-h-[calc(100dvh-2rem)] max-w-[calc(100vw-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0 sm:max-w-2xl"
        enterConfirms={!isBlocking}
        data-testid="workflow-cycle-guard-dialog"
      >
        <AlertDialogHeader className="place-items-start p-4 pb-3 text-left sm:p-6 sm:pb-4">
          <AlertDialogTitle className="text-lg">
            {isBlocking ? t("workflows:workflowCycleBlocked") : t("workflows:confirmWorkflowCycle")}
          </AlertDialogTitle>
          <AlertDialogDescription className="text-left text-sm">
            {isBlocking
              ? t("workflows:workflowCycleBlockedDescription")
              : t("workflows:confirmWorkflowCycleDescription")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div
          className="min-h-0 overflow-x-hidden overflow-y-auto px-4 pb-4 sm:px-6"
          data-testid="workflow-cycle-guard-scroll"
        >
          <div className="grid min-w-0 gap-3">
            {proposal?.diagnostics.map((diagnostic) => (
              <WorkflowCycleDiagnostic key={diagnostic.identity} diagnostic={diagnostic} />
            ))}
          </div>
        </div>
        <AlertDialogFooter className="border-t bg-background p-4 sm:px-6">
          <AlertDialogCancel className="min-h-12 w-full cursor-pointer sm:w-auto">
            {isBlocking ? t("workflows:returnToWorkflow") : t("common:cancel")}
          </AlertDialogCancel>
          {!isBlocking && (
            <AlertDialogAction
              data-dialog-default-action
              className="min-h-12 w-full cursor-pointer sm:w-auto"
              onClick={handleConfirm}
            >
              {actionLabel}
            </AlertDialogAction>
          )}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
