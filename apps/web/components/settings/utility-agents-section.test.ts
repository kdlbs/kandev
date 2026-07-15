import { describe, expect, it } from "vitest";
import type { UtilityAgent } from "@/lib/api/domains/utility-api";
import { mergeRefreshedUtilityAgents, replaceCustomUtilityAgents } from "./utility-agents-section";

function agent(id: string, builtin: boolean, model: string): UtilityAgent {
  return {
    id,
    name: id,
    description: "",
    builtin,
    enabled: true,
    agent_id: "agent-1",
    model,
    prompt: "",
    created_at: "",
    updated_at: "",
  };
}

describe("replaceCustomUtilityAgents", () => {
  it("refreshes immediate custom resources without replacing built-in drafts", () => {
    const builtinDraft = agent("commit", true, "draft-model");
    const refreshedCustom = agent("custom-new", false, "saved-model");

    expect(
      replaceCustomUtilityAgents(
        [builtinDraft, agent("custom-old", false, "old-model")],
        [refreshedCustom],
      ),
    ).toEqual([builtinDraft, refreshedCustom]);
  });

  it("refreshes saved builtins while preserving unsaved model overrides", () => {
    const baseline = agent("commit", true, "saved-model");
    const draft = { ...baseline, model: "draft-model" };
    const refreshed = { ...baseline, prompt: "updated in dialog", model: "dialog-model" };

    expect(mergeRefreshedUtilityAgents([draft], [baseline], [refreshed])).toEqual([
      { ...refreshed, model: "draft-model" },
    ]);
  });
});
