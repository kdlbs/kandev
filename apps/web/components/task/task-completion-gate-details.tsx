"use client";

import { useState } from "react";
import { TaskCompletionCriteriaSection } from "./task-completion-gate-criteria";
import {
  TaskCompletionGateBlockers,
  TaskCompletionGateHistorySection,
  TaskCompletionOverrideSection,
} from "./task-completion-gate-sections";
import {
  useCompletionCriteriaActions,
  useCompletionEvidenceActions,
  useCompletionOverrideActions,
} from "./use-task-completion-gate-actions";
import type { WorkflowStepperStep } from "@/components/task/workflow-step-disclosure";
import type {
  TaskCompletionGateHistory,
  TaskCompletionGateSnapshot,
} from "@/lib/api/domains/task-completion-gates-api";

type TaskCompletionGateDetailsProps = {
  taskId: string;
  taskUpdatedAt: string;
  snapshot: TaskCompletionGateSnapshot | null;
  history: TaskCompletionGateHistory[];
  completionSteps: WorkflowStepperStep[];
  error: string | null;
  onRefresh: () => Promise<void>;
  onOverride: (stepId: string, reason: string, revision: number) => Promise<boolean>;
};

export function TaskCompletionGateDetails(props: TaskCompletionGateDetailsProps) {
  const [localError, setLocalError] = useState<string | null>(null);
  const onError = (message: string) => setLocalError(message);
  const criteria = useCompletionCriteriaActions({
    taskId: props.taskId,
    snapshot: props.snapshot,
    onRefresh: props.onRefresh,
    onError,
  });
  const evidence = useCompletionEvidenceActions({
    taskId: props.taskId,
    snapshot: props.snapshot,
    onRefresh: props.onRefresh,
    onError,
  });
  const override = useCompletionOverrideActions({
    snapshot: props.snapshot,
    onOverride: props.onOverride,
  });

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 py-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
      <TaskCompletionGateBlockers snapshot={props.snapshot} />
      <TaskCompletionCriteriaSection
        snapshot={props.snapshot}
        taskUpdatedAt={props.taskUpdatedAt}
        draft={criteria.draft}
        onDraftChange={criteria.onDraftChange}
        savingCriteria={criteria.saving}
        savingEvidence={evidence.savingCriterionId}
        changed={criteria.changed}
        weakening={criteria.weakening}
        confirmWeakening={criteria.confirmWeakening}
        onConfirmWeakeningChange={criteria.onConfirmWeakeningChange}
        confirmationReason={criteria.confirmationReason}
        onConfirmationReasonChange={criteria.onConfirmationReasonChange}
        onSaveCriteria={() => void criteria.onSave()}
        onSaveEvidence={evidence.onSave}
      />
      <TaskCompletionOverrideSection
        visible={props.snapshot?.blocked === true}
        completionSteps={props.completionSteps}
        selectedStep={override.selectedStep}
        reason={override.reason}
        saving={override.saving}
        onStepChange={override.onStepChange}
        onReasonChange={override.onReasonChange}
        onSubmit={() => void override.submit()}
      />
      <TaskCompletionGateHistorySection history={props.history} />
      {(props.error || localError) && (
        <p
          className="text-sm text-destructive"
          role="alert"
          data-testid="task-completion-gate-error"
        >
          {props.error ?? localError}
        </p>
      )}
    </div>
  );
}
