import { describe, expect, it } from "vitest";
import { groupAuthorExclusionWarnings } from "./author-exclusion-warning-groups";
import type { AuthorExclusionWarning } from "@/lib/types/author-exclusion";

const CLAUDE = "claude-acp";

function single(workflow: string, provider = CLAUDE): AuthorExclusionWarning {
  return {
    code: "single_provider",
    workflow_name: workflow,
    step_name: "Review",
    role: "reviewer",
    provider_id: provider,
  };
}

describe("groupAuthorExclusionWarnings", () => {
  it("merges warnings with the same code, provider, and agent in first-seen order", () => {
    const groups = groupAuthorExclusionWarnings([
      single("Alpha"),
      {
        code: "agent_single_provider",
        workflow_name: "Alpha",
        step_name: "Review",
        role: "reviewer",
        provider_id: "codex-acp",
        agent_name: "QA",
      },
      single("Beta"),
      single("Gamma", "codex-acp"),
    ]);

    expect(groups.map((g) => [g.code, g.providerId, g.agentName, g.steps.length])).toEqual([
      ["single_provider", CLAUDE, "", 2],
      ["agent_single_provider", "codex-acp", "QA", 1],
      ["single_provider", "codex-acp", "", 1],
    ]);
    expect(groups[0].steps).toEqual([
      { workflowName: "Alpha", stepName: "Review", role: "reviewer" },
      { workflowName: "Beta", stepName: "Review", role: "reviewer" },
    ]);
  });

  it("keeps a check_failed warning as a group without steps", () => {
    const groups = groupAuthorExclusionWarnings([{ code: "check_failed" }]);
    expect(groups).toHaveLength(1);
    expect(groups[0].steps).toEqual([]);
  });

  it("gives each group a distinct key", () => {
    const groups = groupAuthorExclusionWarnings([single("Alpha"), single("Beta", "codex-acp")]);
    expect(new Set(groups.map((g) => g.key)).size).toBe(2);
  });
});
