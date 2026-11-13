import type { AgentProfileOption } from "@/lib/state/slices";
import type {
  StepType,
  TaskCreateWorkflowStepsResult,
} from "@/components/task-create-dialog-types";
import { t } from "@/lib/i18n";
import {
  buildWorkflowAgentOverrideRows,
  type WorkflowAgentOverrideOption,
  type WorkflowAgentOverrideRow,
} from "@/components/task-create-dialog-workflow-agent-overrides";

export type WorkflowAgentOverrideValidation = {
  rows: WorkflowAgentOverrideRow[];
  options: WorkflowAgentOverrideOption[];
  loading: boolean;
  error: boolean;
  invalid: boolean;
  blockedReason?: string;
};

function overrideSteps(steps: readonly StepType[]) {
  return steps.map((step, index) => ({
    id: step.id,
    title: step.title,
    position: step.position ?? index,
    agent_profile_id: step.agent_profile_id,
    session_target: step.session_target ?? null,
  }));
}

function blockedReasonForValidation(args: {
  isCreateMode: boolean;
  hasSelectedOverride: boolean;
  error: boolean;
  loading: boolean;
  invalid: boolean;
}) {
  if (!args.isCreateMode) return undefined;
  if (!args.hasSelectedOverride) return undefined;
  if (args.error) return t("task:workflowAgentsLoadError");
  if (args.loading) return t("task:workflowAgentsLoading");
  return args.invalid ? t("task:workflowAgentsUnavailable") : undefined;
}

export function buildWorkflowAgentOverrideValidation(args: {
  selectedWorkflowSteps: TaskCreateWorkflowStepsResult;
  profiles: readonly AgentProfileOption[];
  replacementOptions: readonly WorkflowAgentOverrideOption[];
  overrides: Readonly<Record<string, string>>;
  isCreateMode: boolean;
}): WorkflowAgentOverrideValidation {
  const { status, steps } = args.selectedWorkflowSteps;
  const rows = buildWorkflowAgentOverrideRows({
    steps: overrideSteps(steps),
    profiles: args.profiles,
    replacementOptions: args.replacementOptions,
    overrides: args.overrides,
  });
  const hasSelectedOverride = Object.values(args.overrides ?? {}).some(
    (replacementProfileId) => replacementProfileId.trim() !== "",
  );
  const loading = status === "loading";
  const error = status === "error";
  const invalid = status === "success" && rows.some((row) => !row.replacementAvailable);
  return {
    rows,
    options: args.replacementOptions as WorkflowAgentOverrideOption[],
    loading,
    error,
    invalid,
    blockedReason: blockedReasonForValidation({
      isCreateMode: args.isCreateMode,
      hasSelectedOverride,
      error,
      loading,
      invalid,
    }),
  };
}
