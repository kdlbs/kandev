"use client";

import { useRef } from "react";
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

const PROMPT_SOURCE_TEXT = {
  task_description: (stepName: string) =>
    `"${stepName}" has no step prompt, so re-entering it sends the task description.`,
  step_prompt_with_task_description: (stepName: string) =>
    `Re-entering "${stepName}" sends its rendered step prompt including the task description.`,
  step_prompt: (stepName: string) =>
    `Re-entering "${stepName}" sends its step prompt instead of the task description.`,
} as const;

function CycleHop({ hop, index }: { hop: WorkflowReplayCycleHop; index: number }) {
  return (
    <li className="min-w-0 rounded-md border bg-background/60 p-2.5">
      <div className="grid min-w-0 gap-2 sm:grid-cols-[minmax(7rem,1fr)_auto_minmax(7rem,1fr)] sm:items-center">
        <span className="min-w-0 break-words font-medium">{hop.sourceStepName}</span>
        <div className="flex min-w-0 flex-wrap items-center gap-1.5 text-muted-foreground">
          <code className="break-all rounded bg-muted px-1 py-0.5">{hop.trigger}</code>
          <code className="break-all rounded bg-muted px-1 py-0.5">{hop.actionKind}</code>
          <IconArrowRight className="size-3.5 shrink-0" aria-hidden="true" />
        </div>
        <span className="min-w-0 break-words font-medium">{hop.destinationStepName}</span>
      </div>
      {hop.requiresUserInvolvement && (
        <div className="mt-2 flex items-center gap-1.5 text-xs font-medium text-foreground">
          <IconUser className="size-3.5 shrink-0" aria-hidden="true" />
          <span>User action required</span>
        </div>
      )}
      <span className="sr-only">Hop {index + 1}</span>
    </li>
  );
}

export function WorkflowCycleDiagnostic({
  diagnostic,
}: {
  diagnostic: WorkflowReplayCycleDiagnostic;
}) {
  const isBlocking = diagnostic.severity === "blocking";
  const promptText = PROMPT_SOURCE_TEXT[diagnostic.promptSource](diagnostic.autoStartStepName);

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
        {isBlocking ? "Automatic workflow cycle" : "Potential repeated agent run"}
      </AlertTitle>
      <AlertDescription className="min-w-0 space-y-3 text-left text-sm text-pretty">
        <p>
          {isBlocking
            ? `This path re-enters "${diagnostic.autoStartStepName}" and can start the agent again without another user action.`
            : `This path can re-enter "${diagnostic.autoStartStepName}" and start the agent again after a user action.`}
        </p>
        <ol
          aria-label={`Replay path for ${diagnostic.autoStartStepName}`}
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
  const isBlocking = proposal?.severity === "blocking";
  const actionLabel = proposal?.intent === "create" ? "Create anyway" : "Apply anyway";
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
        className="max-h-[calc(100dvh-2rem)] max-w-[calc(100vw-2rem)] overflow-x-hidden overflow-y-auto sm:max-w-2xl"
        enterConfirms={!isBlocking}
        data-testid="workflow-cycle-guard-dialog"
      >
        <AlertDialogHeader>
          <AlertDialogTitle>
            {isBlocking ? "Workflow cycle blocked" : "Confirm workflow cycle"}
          </AlertDialogTitle>
          <AlertDialogDescription className="text-left">
            {isBlocking
              ? "Change the workflow steps to remove the automatic cycle before continuing."
              : "Review the repeated agent run before continuing."}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="grid min-w-0 gap-3">
          {proposal?.diagnostics.map((diagnostic) => (
            <WorkflowCycleDiagnostic key={diagnostic.identity} diagnostic={diagnostic} />
          ))}
        </div>
        <AlertDialogFooter className="sticky bottom-0 bg-background pt-1">
          <AlertDialogCancel className="min-h-12 w-full cursor-pointer sm:w-auto">
            {isBlocking ? "Return to workflow" : "Cancel"}
          </AlertDialogCancel>
          {!isBlocking && (
            <AlertDialogAction
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
