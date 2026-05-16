"use client";

import { useMemo } from "react";
import { useRouter } from "next/navigation";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import type { Task, Workflow, WorkflowStep } from "@/lib/types/http";
import type { JiraTicket } from "@/lib/types/jira";
import type { JiraTaskPreset } from "./presets";

export type JiraLaunchPayload = {
  ticket: JiraTicket;
  preset: JiraTaskPreset;
};

type QuickTaskLauncherProps = {
  workspaceId: string | null;
  workflows: Workflow[];
  steps: WorkflowStep[];
  payload: JiraLaunchPayload | null;
  onClose: () => void;
};

function buildDialogState(payload: JiraLaunchPayload) {
  const { ticket, preset } = payload;
  const title = `${ticket.key}: ${ticket.summary}`;
  const description = preset.prompt({
    url: ticket.url,
    key: ticket.key,
    title: ticket.summary,
    description: ticket.description,
  });
  return { title, description };
}

export function QuickTaskLauncher({
  workspaceId,
  workflows,
  steps,
  payload,
  onClose,
}: QuickTaskLauncherProps) {
  const router = useRouter();

  const defaultWorkflow = workflows[0];
  const sortedStepsForWorkflow = useMemo(
    () =>
      steps
        .filter((s) => s.workflow_id === defaultWorkflow?.id)
        .sort((a, b) => a.position - b.position),
    [steps, defaultWorkflow],
  );
  const defaultStep = sortedStepsForWorkflow[0];
  const stepsForWorkflow = useMemo(
    () => sortedStepsForWorkflow.map((s) => ({ id: s.id, title: s.name, events: s.events })),
    [sortedStepsForWorkflow],
  );
  const dialog = useMemo(() => (payload ? buildDialogState(payload) : null), [payload]);

  const handleOpenChange = (open: boolean) => {
    if (!open) onClose();
  };
  const handleSuccess = (task: Task) => {
    onClose();
    router.push(`/tasks/${task.id}`);
  };

  if (!workspaceId || !defaultWorkflow || !defaultStep || !dialog) return null;

  return (
    <TaskCreateDialog
      open={true}
      onOpenChange={handleOpenChange}
      mode="create"
      workspaceId={workspaceId}
      workflowId={defaultWorkflow.id}
      defaultStepId={defaultStep.id}
      steps={stepsForWorkflow}
      initialValues={{ title: dialog.title, description: dialog.description }}
      onSuccess={handleSuccess}
    />
  );
}
