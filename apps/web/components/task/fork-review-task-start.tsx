"use client";

import { useRef, useState } from "react";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import type { WorkflowStepperStep } from "./workflow-step-disclosure";
import type { Task } from "@/lib/types/http";
import { ForkReviewStartWarning } from "./fork-review-start-warning";

export function ForkReviewTaskStart({
  task,
  workflowSteps,
}: {
  task: Task;
  workflowSteps: WorkflowStepperStep[];
}) {
  const [open, setOpen] = useState(false);
  const startRef = useRef<HTMLButtonElement>(null);
  const taskValues = {
    title: task.title,
    description: task.description,
    state: task.state,
    repositories: task.repositories,
  };
  return (
    <div className="@container mx-auto mt-auto w-full max-w-2xl px-3 py-3">
      <ForkReviewStartWarning inChat onStart={() => setOpen(true)} startRef={startRef} />
      <TaskCreateDialog
        open={open}
        onOpenChange={setOpen}
        mode="edit"
        focusReturnRef={startRef}
        workspaceId={task.workspace_id}
        workflowId={task.workflow_id}
        defaultStepId={task.workflow_step_id}
        steps={workflowSteps.map((step) => ({ ...step, title: step.name }))}
        lockedFields={{ workflow: true }}
        initialValues={taskValues}
        editingTask={{
          ...taskValues,
          id: task.id,
          workflowStepId: task.workflow_step_id,
          manualStartRequired: true,
          primaryExecutorProfileId: task.primary_executor_profile_id ?? undefined,
          runnerEditable: task.runner_editable,
          runnerIneligibleReason: task.runner_ineligible_reason,
        }}
      />
    </div>
  );
}
