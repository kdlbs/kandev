import type { AgentProfileOption } from "@/lib/state/slices";
import type { WorkflowSessionTarget } from "@/lib/types/http";

export type WorkflowAgentOverrideStep = {
  id: string;
  title: string;
  position: number;
  agent_profile_id?: string;
  session_target?: WorkflowSessionTarget | null;
};

export type WorkflowAgentOverrideOption = {
  value: string;
  label: string;
  renderLabel: () => React.ReactNode;
};

export type WorkflowAgentOverrideRow = {
  sourceProfileId: string;
  sourceLabel: string;
  sourceAgentName: string | null;
  stepIds: string[];
  stepNames: string[];
  replacementProfileId: string;
  replacementLabel: string | null;
  replacementAvailable: boolean;
};

export function normalizeWorkflowAgentOverrides(
  overrides: Readonly<Record<string, string>>,
): Record<string, string> {
  return Object.fromEntries(
    Object.entries(overrides)
      .map(([sourceProfileId, replacementProfileId]) => [
        sourceProfileId.trim(),
        replacementProfileId.trim(),
      ])
      .filter(
        ([sourceProfileId, replacementProfileId]) =>
          sourceProfileId !== "" &&
          replacementProfileId !== "" &&
          sourceProfileId !== replacementProfileId,
      ),
  );
}

export function buildWorkflowAgentOverrideRows(args: {
  steps: readonly WorkflowAgentOverrideStep[];
  profiles: readonly AgentProfileOption[];
  replacementOptions: readonly WorkflowAgentOverrideOption[];
  overrides: Readonly<Record<string, string>>;
}): WorkflowAgentOverrideRow[] {
  const profilesById = new Map(args.profiles.map((profile) => [profile.id, profile]));
  const replacementOptionIds = new Set(args.replacementOptions.map((option) => option.value));
  const stepsById = new Map(args.steps.map((step) => [step.id, step]));
  const groups = new Map<
    string,
    { stepIds: string[]; stepNames: string[]; firstPosition: number }
  >();

  const resolveFixedSourceProfile = (stepId: string, visited = new Set<string>()): string => {
    if (visited.has(stepId)) return "";
    const step = stepsById.get(stepId);
    if (!step) return "";
    const nextVisited = new Set(visited).add(stepId);
    if (!step.session_target) return step.agent_profile_id?.trim() ?? "";
    if (step.session_target.kind !== "step") return "";
    return resolveFixedSourceProfile(step.session_target.step_id, nextVisited);
  };

  for (const step of [...args.steps].sort(
    (a, b) => a.position - b.position || a.id.localeCompare(b.id),
  )) {
    const sourceProfileId = step.session_target
      ? resolveFixedSourceProfile(step.id)
      : (step.agent_profile_id?.trim() ?? "");
    if (!sourceProfileId) continue;
    const group = groups.get(sourceProfileId) ?? {
      stepIds: [],
      stepNames: [],
      firstPosition: step.position,
    };
    group.stepIds.push(step.id);
    group.stepNames.push(step.title);
    groups.set(sourceProfileId, group);
  }

  return [...groups.entries()]
    .sort(([, left], [, right]) => left.firstPosition - right.firstPosition)
    .map(([sourceProfileId, group]) => {
      const sourceProfile = profilesById.get(sourceProfileId);
      const replacementProfileId = args.overrides[sourceProfileId]?.trim() ?? "";
      const replacementProfile = replacementProfileId
        ? profilesById.get(replacementProfileId)
        : undefined;
      return {
        sourceProfileId,
        sourceLabel: sourceProfile?.label ?? sourceProfileId,
        sourceAgentName: sourceProfile?.agent_name ?? null,
        stepIds: group.stepIds,
        stepNames: group.stepNames,
        replacementProfileId,
        replacementLabel: replacementProfile?.label ?? (replacementProfileId || null),
        replacementAvailable:
          replacementProfileId === "" ||
          (replacementProfile?.enabled !== false && replacementOptionIds.has(replacementProfileId)),
      };
    });
}
