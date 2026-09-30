import type { WorkflowChangePayload } from "@/lib/api/domains/kanban-api";
import type { Task } from "@/lib/types/http";

export function normalizeChangeWorkflowOverrides(
  overrides: Readonly<Record<string, string>>,
): Record<string, string> {
  return Object.fromEntries(
    Object.entries(overrides)
      .map(([source, replacement]) => [source.trim(), replacement.trim()])
      .filter(([source, replacement]) => source && replacement && source !== replacement)
      .sort(([left], [right]) => left.localeCompare(right)),
  );
}

export function buildWorkflowChangePayload(
  task: Task,
  overrides: Readonly<Record<string, string>>,
): WorkflowChangePayload {
  return {
    expected_workflow_id: task.workflow_id,
    expected_step_id: task.workflow_step_id,
    expected_updated_at: task.updated_at,
    agent_overrides: normalizeChangeWorkflowOverrides(overrides),
  };
}

export function taskMatchesWorkflowChange(
  task: Task,
  workflowId: string,
  stepId: string,
  expectedOverrides: Readonly<Record<string, string>>,
): boolean {
  if (task.workflow_id !== workflowId || task.workflow_step_id !== stepId) return false;
  const persisted = task.workflow_agent_overrides;
  const actualOverrides =
    persisted?.workflow_id === workflowId
      ? Object.fromEntries(
          persisted.steps.map((binding) => [
            binding.source_profile_id,
            binding.replacement_profile_id,
          ]),
        )
      : {};
  return (
    JSON.stringify(normalizeChangeWorkflowOverrides(actualOverrides)) ===
    JSON.stringify(normalizeChangeWorkflowOverrides(expectedOverrides))
  );
}
