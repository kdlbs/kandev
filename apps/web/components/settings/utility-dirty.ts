import type { UtilityAgent } from "@/lib/api/domains/utility-api";

export function isUtilityAgentDirty(draft: UtilityAgent, saved: UtilityAgent | undefined): boolean {
  return (
    !saved ||
    draft.agent_id !== saved.agent_id ||
    draft.model !== saved.model ||
    draft.enabled !== saved.enabled
  );
}
