"use client";

import { useMemo } from "react";
import { useRouter } from "next/navigation";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import type { Task, Workflow, WorkflowStep } from "@/lib/types/http";
import type { LinearIssue } from "@/lib/types/linear";

type QuickTaskLauncherProps = {
  workspaceId: string | null;
  workflows: Workflow[];
  steps: WorkflowStep[];
  issue: LinearIssue | null;
  onClose: () => void;
};

// Build the prefilled title/description for a Linear-launched task. Mirrors
// the Jira prompt shape so users coming from either integration get a
// consistent skeleton.
function buildDialogState(issue: LinearIssue) {
  const title = `${issue.identifier}: ${issue.title}`;
  const description = [
    `Linear issue: ${issue.identifier}`,
    `URL: ${issue.url}`,
    "",
    "Title:",
    issue.title,
    "",
    "Description:",
    issue.description?.trim() || "(no description)",
  ].join("\n");
  return { title, description };
}

// Renders the TaskCreateDialog prefilled from a Linear issue. Hidden until an
// `issue` is supplied, which keeps the dialog a single React tree per page.
export function LinearQuickTaskLauncher({
  workspaceId,
  workflows,
  steps,
  issue,
  onClose,
}: QuickTaskLauncherProps) {
  const router = useRouter();

  // Default to the first workflow that actually has steps, not just
  // workflows[0] — otherwise an empty first workflow blocks Start task even
  // when other workflows would work fine. Bundling the lookup, the sort and
  // the dialog projection into one memo keeps the launcher's hook list flat
  // and avoids cascading useMemo deps.
  const launchData = useMemo(() => {
    for (const wf of workflows) {
      const wfSteps = steps
        .filter((s) => s.workflow_id === wf.id)
        .sort((a, b) => a.position - b.position);
      if (wfSteps.length > 0) {
        return {
          defaultWorkflow: wf as Workflow | undefined,
          defaultStep: wfSteps[0],
          stepsForWorkflow: wfSteps.map((s) => ({
            id: s.id,
            title: s.name,
            events: s.events,
          })),
        };
      }
    }
    return {
      defaultWorkflow: undefined as Workflow | undefined,
      defaultStep: undefined as WorkflowStep | undefined,
      stepsForWorkflow: [] as Array<{
        id: string;
        title: string;
        events: WorkflowStep["events"];
      }>,
    };
  }, [workflows, steps]);
  const { defaultWorkflow, defaultStep, stepsForWorkflow } = launchData;
  const dialog = useMemo(() => (issue ? buildDialogState(issue) : null), [issue]);

  const handleOpenChange = (open: boolean) => {
    if (!open) onClose();
  };
  const handleSuccess = (task: Task) => {
    onClose();
    router.push(`/t/${task.id}`);
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
