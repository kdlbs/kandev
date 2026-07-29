"use client";

import { resolveTaskCreateWorkflowContext } from "@/components/task-create-dialog-defaults";
import { useAppStore } from "@/components/state-provider";

type WorkflowContextProps = {
  workflowId: string | null;
  defaultStepId: string | null;
  lockedFields?: { workflow?: boolean };
};

export function useResolvedTaskCreateWorkflowContext<T extends WorkflowContextProps>(props: T): T {
  const workflows = useAppStore((state) => state.workflows?.items ?? []);
  const context = resolveTaskCreateWorkflowContext(
    props.workflowId,
    props.defaultStepId,
    workflows,
    props.lockedFields?.workflow === true,
  );
  return { ...props, ...context };
}
