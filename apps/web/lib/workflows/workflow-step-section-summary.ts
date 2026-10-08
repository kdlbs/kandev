import type { WorkflowStep } from "@/lib/types/http";
import {
  normalizeWorkflowProfileSessionEndPolicy,
  normalizeWorkflowProfileSessionStartPolicy,
} from "@/lib/types/http";
import type { WorkflowLifecycleTrigger } from "./workflow-action-catalog";

export type StepSection = "agent" | "instructions" | "automation" | "board" | "advanced";
export type SummaryPart = { key: string; values?: Record<string, string | number> };
export type StepSectionSummaries = Record<StepSection, SummaryPart[]>;

export function isStepSectionDirty(
  step: WorkflowStep,
  saved: WorkflowStep | undefined,
  section: StepSection,
): boolean {
  return (
    !saved ||
    JSON.stringify(sectionValues(step, section)) !== JSON.stringify(sectionValues(saved, section))
  );
}

function sectionValues(step: WorkflowStep, section: StepSection): unknown {
  switch (section) {
    case "agent":
      return [
        step.agent_profile_id,
        step.session_target,
        step.profile_session_start_policy,
        step.profile_session_end_policy,
        hasEntryAction(step, "auto_start_agent"),
        hasEntryAction(step, "enable_plan_mode"),
      ];
    case "instructions":
      return step.prompt ?? "";
    case "automation":
      return step.events;
    case "board":
      return [
        step.is_start_step,
        step.allow_manual_move,
        step.show_in_command_panel,
        step.wip_limit,
        step.pull_from_step_id,
      ];
    case "advanced":
      return [
        step.auto_advance_requires_signal,
        step.cancel_triggers_turn_complete,
        step.complete_task_on_enter,
        step.auto_archive_after_hours,
        step.events?.on_enter?.filter((action) =>
          ["reset_agent_context", "configure_session"].includes(action.type),
        ),
      ];
  }
}

export function buildStepSectionSummaries(
  step: WorkflowStep,
  steps: WorkflowStep[],
  profiles: { id: string; label: string }[],
): StepSectionSummaries {
  const prompt = step.prompt?.trim().replace(/\s+/g, " ");
  return {
    agent: agentSummary(step, steps, profiles),
    instructions: [
      prompt
        ? { key: "workflows:stepInstructionsPreview", values: { prompt } }
        : { key: "workflows:noStepInstructions" },
    ],
    automation: automationSummary(step, steps),
    board: boardSummary(step, steps),
    advanced: advancedSummary(step),
  };
}

function agentSummary(
  step: WorkflowStep,
  steps: WorkflowStep[],
  profiles: { id: string; label: string }[],
): SummaryPart[] {
  const parts: SummaryPart[] = [];
  if (step.session_target) {
    const target = step.session_target;
    const source =
      target.kind === "step" ? steps.find((item) => item.id === target.step_id) : undefined;
    if (target.kind === "initial") parts.push({ key: "workflows:initialAgentSession" });
    else if (source)
      parts.push({ key: "workflows:stepSessionSummary", values: { step: source.name } });
    else parts.push({ key: "workflows:stepDestinationUnavailable" });
  } else if (step.agent_profile_id) {
    const profile = profiles.find((item) => item.id === step.agent_profile_id);
    parts.push(
      profile
        ? { key: "workflows:profileValue", values: { profile: profile.label } }
        : { key: "workflows:profileUnavailable" },
    );
    parts.push({
      key:
        normalizeWorkflowProfileSessionStartPolicy(step.profile_session_start_policy) === "new"
          ? "workflows:profileSessionStartNewShort"
          : "workflows:profileSessionStartReuseShort",
    });
    parts.push({
      key:
        normalizeWorkflowProfileSessionEndPolicy(step.profile_session_end_policy) === "complete"
          ? "workflows:profileSessionEndCompleteShort"
          : "workflows:profileSessionEndParkShort",
    });
  } else parts.push({ key: "workflows:inheritedStepAgent" });
  parts.push({
    key: hasEntryAction(step, "auto_start_agent")
      ? "workflows:autoStartAgent"
      : "workflows:manualAgentStart",
  });
  if (hasEntryAction(step, "enable_plan_mode")) parts.push({ key: "workflows:planMode" });
  return parts;
}

const TRIGGERS: WorkflowLifecycleTrigger[] = [
  "on_enter",
  "on_turn_start",
  "on_turn_complete",
  "on_children_completed",
  "on_exit",
];

function automationSummary(step: WorkflowStep, steps: WorkflowStep[]): SummaryPart[] {
  const count = TRIGGERS.reduce(
    (total, trigger) => total + (step.events?.[trigger]?.length ?? 0),
    0,
  );
  const parts: SummaryPart[] = [{ key: "workflows:stepActionCount", values: { count } }];
  const transition = step.events?.on_turn_complete?.find((action) =>
    ["move_to_step", "move_to_next", "move_to_previous"].includes(action.type),
  );
  if (!transition) return parts;
  const ordered = [...steps].sort((a, b) => a.position - b.position);
  const index = ordered.findIndex((item) => item.id === step.id);
  const destination =
    transition.type === "move_to_step"
      ? steps.find((item) => item.id === transition.config?.step_id)
      : ordered[index + (transition.type === "move_to_next" ? 1 : -1)];
  parts.push(
    destination
      ? { key: "workflows:stepCompletionDestination", values: { step: destination.name } }
      : { key: "workflows:stepDestinationUnavailable" },
  );
  return parts;
}

function boardSummary(step: WorkflowStep, steps: WorkflowStep[]): SummaryPart[] {
  const parts: SummaryPart[] = [];
  if (step.is_start_step) parts.push({ key: "workflows:startStep" });
  parts.push({
    key:
      step.allow_manual_move !== false
        ? "workflows:allowManualMove"
        : "workflows:manualMovesDisabled",
  });
  if (step.show_in_command_panel !== false) parts.push({ key: "workflows:showInCommandPanel" });
  parts.push(
    (step.wip_limit ?? 0) > 0
      ? { key: "workflows:stepWipSummary", values: { count: step.wip_limit! } }
      : { key: "workflows:noWipLimit" },
  );
  if (step.pull_from_step_id) {
    const source = steps.find((item) => item.id === step.pull_from_step_id);
    parts.push(
      source
        ? { key: "workflows:stepPullSummary", values: { step: source.name } }
        : { key: "workflows:stepDestinationUnavailable" },
    );
  }
  return parts;
}

function advancedSummary(step: WorkflowStep): SummaryPart[] {
  const parts: SummaryPart[] = [];
  if (hasEntryAction(step, "reset_agent_context"))
    parts.push({ key: "workflows:resetAgentContext" });
  if (hasEntryAction(step, "configure_session"))
    parts.push({ key: "workflows:overrideOriginalSessionOptions" });
  if (step.auto_advance_requires_signal) parts.push({ key: "workflows:requireCompletionSignal" });
  if (step.cancel_triggers_turn_complete)
    parts.push({ key: "workflows:runCompletionActionsWhenTurnCancelled" });
  if (step.complete_task_on_enter) parts.push({ key: "workflows:completeTaskOnEnter" });
  if ((step.auto_archive_after_hours ?? 0) > 0)
    parts.push({
      key: "workflows:stepArchiveSummary",
      values: { count: step.auto_archive_after_hours! },
    });
  return parts.length ? parts : [{ key: "workflows:defaultStepSettings" }];
}

function hasEntryAction(step: WorkflowStep, type: string): boolean {
  return step.events?.on_enter?.some((action) => action.type === type) ?? false;
}
