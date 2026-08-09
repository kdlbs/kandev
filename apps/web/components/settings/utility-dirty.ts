import type { UtilityAgent } from "@/lib/api/domains/utility-api";

export function isUtilityAgentDirty(draft: UtilityAgent, saved: UtilityAgent | undefined): boolean {
  return (
    !saved ||
    draft.agent_id !== saved.agent_id ||
    draft.model !== saved.model ||
    draft.agent_profile_id !== saved.agent_profile_id ||
    draft.profile_binding_state !== saved.profile_binding_state ||
    draft.enabled !== saved.enabled
  );
}
