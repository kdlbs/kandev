import type {
  Coordinator,
  CreateCoordinatorRequest,
  PatchCoordinatorRequest,
} from "@/lib/api/domains/coordinator-api";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Executor } from "@/lib/types/http";

export type CoordinatorFormState = {
  name: string;
  agentProfileId: string;
  executorProfileId: string;
  context: string;
};

export function coordinatorFormFromRecord(coordinator: Coordinator): CoordinatorFormState {
  return {
    name: coordinator.name,
    agentProfileId: coordinator.agent_profile_id,
    executorProfileId: coordinator.executor_profile_id,
    context: coordinator.context,
  };
}

export function buildCreateCoordinatorPayload(
  form: CoordinatorFormState,
): CreateCoordinatorRequest {
  return {
    name: form.name.trim(),
    agent_profile_id: form.agentProfileId,
    executor_profile_id: form.executorProfileId,
    context: form.context,
  };
}

// Sends only what changed against the saved form (design B6), so a PATCH
// never re-asserts a field the server already merged.
export function buildPatchCoordinatorPayload(
  form: CoordinatorFormState,
  saved: CoordinatorFormState,
): PatchCoordinatorRequest {
  const patch: PatchCoordinatorRequest = {};
  const trimmedName = form.name.trim();
  if (trimmedName !== saved.name) patch.name = trimmedName;
  if (form.agentProfileId !== saved.agentProfileId) patch.agent_profile_id = form.agentProfileId;
  if (form.executorProfileId !== saved.executorProfileId) {
    patch.executor_profile_id = form.executorProfileId;
  }
  if (form.context !== saved.context) patch.context = form.context;
  return patch;
}

// The Add page preselects the workspace's default agent profile only when it
// still exists and is not CLI-passthrough (build decision B4); a coordinator
// can never save a passthrough profile, so preselecting one it would reject
// on submit would be worse than leaving the field empty.
export function resolveDefaultAgentProfileId(
  defaultAgentProfileId: string | null | undefined,
  agentProfiles: readonly AgentProfileOption[],
): string {
  if (!defaultAgentProfileId) return "";
  const profile = agentProfiles.find((candidate) => candidate.id === defaultAgentProfileId);
  if (!profile || profile.cli_passthrough) return "";
  return profile.id;
}

// The workspace's `default_executor_id` names an Executor, not an Executor
// Profile; mirroring `profileForTaskExecutor` (use-change-workflow.ts), the
// default profile is that executor's first one.
export function resolveDefaultExecutorProfileId(
  defaultExecutorId: string | null | undefined,
  executors: readonly Executor[],
): string {
  if (!defaultExecutorId) return "";
  const executor = executors.find((candidate) => candidate.id === defaultExecutorId);
  return executor?.profiles?.[0]?.id ?? "";
}
