import { describe, expect, it } from "vitest";
import type { UtilityAgent } from "@/lib/api/domains/utility-api";
import {
  isUtilityAgentDirty,
  mergeRefreshedUtilityAgents,
  replaceCustomUtilityAgents,
} from "./utility-agents-section";

const DRAFT_MODEL = "draft-model";

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
  it("compares only draftable built-in fields", () => {
    const baseline = agent("commit", true, "saved-model");

    expect(isUtilityAgentDirty({ ...baseline, description: "refreshed" }, baseline)).toBe(false);
    expect(isUtilityAgentDirty({ ...baseline, enabled: false }, baseline)).toBe(true);
    expect(isUtilityAgentDirty({ ...baseline, model: DRAFT_MODEL }, baseline)).toBe(true);
  });

  it("refreshes immediate custom resources without replacing built-in drafts", () => {
    const builtinDraft = agent("commit", true, DRAFT_MODEL);
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
    const draft = { ...baseline, model: DRAFT_MODEL };
    const refreshed = { ...baseline, prompt: "updated in dialog", model: "dialog-model" };

    expect(mergeRefreshedUtilityAgents([draft], [baseline], [refreshed])).toEqual([
      { ...refreshed, model: DRAFT_MODEL },
    ]);
  });
});
