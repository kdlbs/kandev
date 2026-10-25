import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Executor, ExecutorProfile } from "@/lib/types/http";

export function resolveAgentProfileLabel(
  agentProfileId: string,
  profiles: readonly AgentProfileOption[],
  missingLabel: string,
): string {
  return profiles.find((profile) => profile.id === agentProfileId)?.label ?? missingLabel;
}

export function flattenExecutorProfiles(executors: readonly Executor[]): ExecutorProfile[] {
  return executors.flatMap((executor) => executor.profiles ?? []);
}

// The coordinator list route carries no profile statuses (design
// §Validation), so a card resolves executor profile names from whatever is
// already loaded and shows the missing-profile text (build decision B9)
// exactly as it would for an agent profile.
export function resolveExecutorProfileLabel(
  executorProfileId: string,
  executors: readonly Executor[],
  missingLabel: string,
): string {
  const profile = flattenExecutorProfiles(executors).find(
    (candidate) => candidate.id === executorProfileId,
  );
  return profile?.name ?? missingLabel;
}
