import { describe, expect, it } from "vitest";
import type { ExecutionProfileSummary, ProviderProfile } from "@/lib/state/slices/office/types";
import { applyExecutionProfileSelection, profilesForProvider } from "./provider-tier-mapping";

const PROFILES: ExecutionProfileSummary[] = [
  { id: "codex-work", name: "Codex Work", provider_id: "codex-acp", model: "gpt-5.6" },
  { id: "claude-opus", name: "Claude Opus", provider_id: "claude-acp", model: "opus" },
];

describe("provider tier profile selection", () => {
  it("filters the catalogue by logical provider", () => {
    expect(profilesForProvider(PROFILES, "claude-acp")).toEqual([PROFILES[1]]);
  });

  it("stores the execution profile and derives the model snapshot", () => {
    const current: ProviderProfile = {
      tier_map: { balanced: "stale-model" },
      tier_profile_ids: { frontier: "legacy-frontier" },
      mode: "legacy-mode",
      flags: ["--legacy"],
      env: { LEGACY: "1" },
    };
    expect(applyExecutionProfileSelection(current, PROFILES, "balanced", "claude-opus")).toEqual({
      tier_map: { balanced: "opus" },
      execution_profile_ids: {
        frontier: "legacy-frontier",
        balanced: "claude-opus",
      },
      tier_profile_ids: undefined,
      mode: undefined,
      flags: undefined,
      env: undefined,
    });
  });
});
