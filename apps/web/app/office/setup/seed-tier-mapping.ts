import type { Tier } from "@/lib/state/slices/office/types";

// Per-provider seed mapping from a chosen model id to a tier. The
// backend Phase 6 onboarding seed should mirror this; the UI uses it
// only to suggest a default tier on the picker step. The user can
// override before continuing.
const SEED_BY_PROVIDER: Record<string, Array<{ keyword: RegExp; tier: Tier }>> = {
  "claude-acp": [
    { keyword: /opus/i, tier: "frontier" },
    { keyword: /sonnet/i, tier: "balanced" },
    { keyword: /haiku/i, tier: "economy" },
  ],
  "codex-acp": [
    { keyword: /(gpt-?5\.5|o4-?mini|o3)/i, tier: "frontier" },
    { keyword: /(gpt-?5\.4|gpt-?4\.1)/i, tier: "balanced" },
    { keyword: /(gpt-?5\.3-?mini|gpt-?4o-?mini|mini)/i, tier: "economy" },
  ],
};

const FALLBACK_TIER: Tier = "balanced";

export function seedTier(agentId: string | undefined, model: string | undefined): Tier {
  if (!agentId || !model) return FALLBACK_TIER;
  const rules = SEED_BY_PROVIDER[agentId];
  if (!rules) return FALLBACK_TIER;
  for (const r of rules) {
    if (r.keyword.test(model)) return r.tier;
  }
  return FALLBACK_TIER;
}
