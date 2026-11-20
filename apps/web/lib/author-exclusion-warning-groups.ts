import type {
  AuthorExclusionWarning,
  AuthorExclusionWarningCode,
} from "@/lib/types/author-exclusion";

export type AuthorExclusionAffectedStep = {
  workflowName: string;
  stepName: string;
  role: string;
};

/** Warnings that share a cause and a fix: one code, provider, and agent. */
export type AuthorExclusionWarningGroup = {
  key: string;
  code: AuthorExclusionWarningCode;
  providerId: string;
  agentName: string;
  steps: AuthorExclusionAffectedStep[];
};

/**
 * Groups setup warnings by code, provider, and agent, in first-seen order, so
 * a cause shared by many steps is shown once with the steps listed under it.
 */
export function groupAuthorExclusionWarnings(
  warnings: AuthorExclusionWarning[],
): AuthorExclusionWarningGroup[] {
  const groups = new Map<string, AuthorExclusionWarningGroup>();
  for (const w of warnings) {
    const providerId = w.provider_id ?? "";
    const agentName = w.agent_name ?? "";
    const key = JSON.stringify([w.code, providerId, agentName]);
    let group = groups.get(key);
    if (!group) {
      group = { key, code: w.code, providerId, agentName, steps: [] };
      groups.set(key, group);
    }
    if (w.workflow_name) {
      group.steps.push({
        workflowName: w.workflow_name,
        stepName: w.step_name ?? "",
        role: w.role ?? "",
      });
    }
  }
  return [...groups.values()];
}
