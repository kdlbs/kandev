export type AgentProfileFallbackState =
  | { kind: "none" }
  | { kind: "exact" }
  | { kind: "next" }
  | { kind: "model"; model: string };

export function classifyAgentProfileFallback(profile: {
  autoFallback?: boolean;
  fallbackModel?: string;
  requireExactModel?: boolean;
}): AgentProfileFallbackState {
  if (profile.requireExactModel) return { kind: "exact" };
  if (profile.autoFallback) return { kind: "next" };
  if (profile.fallbackModel) return { kind: "model", model: profile.fallbackModel };
  return { kind: "none" };
}
