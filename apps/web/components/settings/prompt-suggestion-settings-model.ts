import type { UtilityAgent } from "@/lib/api/domains/utility-api";

export { SUGGEST_NEXT_PROMPT_AGENT_ID } from "@/lib/prompt-suggestion";

export type PromptSuggestionFallbackStatus = "loading" | "ready" | "no-default" | "needs-repair";

/** Reports whether the fallback utility agent can resolve an agent profile. */
export function resolvePromptSuggestionFallbackStatus(
  agent: UtilityAgent | null,
  defaultProfileId: string,
): PromptSuggestionFallbackStatus {
  if (!agent) return "loading";
  if (agent.profile_binding_state === "unconfigured") return "needs-repair";
  if (agent.profile_binding_state !== "inherit" && agent.agent_profile_id) return "ready";
  return defaultProfileId ? "ready" : "no-default";
}
