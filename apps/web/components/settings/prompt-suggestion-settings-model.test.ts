import { describe, expect, it } from "vitest";
import type { UtilityAgent } from "@/lib/api/domains/utility-api";
import {
  SUGGEST_NEXT_PROMPT_AGENT_ID,
  resolvePromptSuggestionFallbackStatus,
} from "./prompt-suggestion-settings-model";

function agent(overrides: Partial<UtilityAgent> = {}): UtilityAgent {
  return {
    id: SUGGEST_NEXT_PROMPT_AGENT_ID,
    name: "suggest-next-prompt",
    description: "",
    prompt: "",
    agent_id: "claude-acp",
    model: "",
    builtin: true,
    enabled: true,
    created_at: "",
    updated_at: "",
    profile_binding_state: "inherit",
    agent_profile_id: "",
    ...overrides,
  };
}

// @covers AC-UI-PROMPT-SUGGEST-001.6
describe("resolvePromptSuggestionFallbackStatus", () => {
  it("is ready when the built-in inherits and a default profile exists", () => {
    expect(resolvePromptSuggestionFallbackStatus(agent(), "profile-default")).toBe("ready");
  });

  it("reports a missing default when the built-in inherits without a default", () => {
    expect(resolvePromptSuggestionFallbackStatus(agent(), "")).toBe("no-default");
  });

  it("is ready with an explicit profile even without a default", () => {
    const explicit = agent({ profile_binding_state: "explicit", agent_profile_id: "profile-x" });
    expect(resolvePromptSuggestionFallbackStatus(explicit, "")).toBe("ready");
  });

  it("reports repair for an unconfigured binding", () => {
    const broken = agent({ profile_binding_state: "unconfigured", agent_profile_id: "gone" });
    expect(resolvePromptSuggestionFallbackStatus(broken, "profile-default")).toBe("needs-repair");
  });

  it("is loading while the built-in has not been fetched", () => {
    expect(resolvePromptSuggestionFallbackStatus(null, "profile-default")).toBe("loading");
  });
});
