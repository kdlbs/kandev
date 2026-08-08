import { describe, expect, it } from "vitest";
import {
  agentProfileId as toAgentProfileId,
  workflowId as toWorkflowId,
  type Workflow,
} from "@/lib/types/http";
import {
  alignSavedWorkflowsToDraftOrder,
  getWorkflowOrderDirtyIds,
  hasNewerWorkflowMetadata,
} from "./workspace-workflows-client";

function workflow(id: string, name = id): Workflow {
  return {
    id: toWorkflowId(id),
    workspace_id: "workspace-1" as Workflow["workspace_id"],
    name,
    created_at: "",
    updated_at: "",
  };
}

describe("alignSavedWorkflowsToDraftOrder", () => {
  it("replaces a client workflow identity without changing its visible order", () => {
    const existing = workflow("existing");
    const draft = workflow("temp-workflow-1", "Draft");
    const persisted = workflow("persisted", "Draft");

    expect(
      alignSavedWorkflowsToDraftOrder(
        [draft, existing],
        [existing, persisted],
        new Map([[draft.id, persisted.id]]),
      ).map(({ id }) => id),
    ).toEqual([persisted.id, existing.id]);
  });

  it("preserves workflows finalized by earlier save contributors", () => {
    const firstDraft = workflow("temp-workflow-1", "First");
    const secondDraft = workflow("temp-workflow-2", "Second");
    const firstSaved = workflow("persisted-1", "First");
    const secondSaved = workflow("persisted-2", "Second");

    expect(
      alignSavedWorkflowsToDraftOrder(
        [firstDraft, secondDraft],
        [firstSaved, secondSaved],
        new Map([
          [firstDraft.id, firstSaved.id],
          [secondDraft.id, secondSaved.id],
        ]),
      ).map(({ id }) => id),
    ).toEqual([firstSaved.id, secondSaved.id]);
  });
});

describe("getWorkflowOrderDirtyIds", () => {
  it("marks only workflows whose visible position changed", () => {
    const first = workflow("first");
    const second = workflow("second");
    const third = workflow("third");

    expect([...getWorkflowOrderDirtyIds([first, third, second], [first, second, third])]).toEqual([
      third.id,
      second.id,
    ]);
  });

  it("marks a newly inserted workflow as order-dirty", () => {
    const first = workflow("first");
    const draft = workflow("temp-workflow-1");

    expect([...getWorkflowOrderDirtyIds([first, draft], [first])]).toEqual([draft.id]);
  });
});

describe("hasNewerWorkflowMetadata", () => {
  it("treats an empty description as unchanged from an absent saved description", () => {
    // Backend omits `description` when empty (omitempty), so a save response's
    // description is undefined, not "". A draft that was never edited beyond
    // that (or was edited then cleared) must not be treated as "newer" than
    // what was just submitted, or a save reconciliation discards the response.
    const saved: Workflow = { ...workflow("wf-1"), description: undefined };
    const current: Workflow = { ...workflow("wf-1"), description: "" };

    expect(hasNewerWorkflowMetadata(current, saved)).toBe(false);
  });

  it("still detects a real description change", () => {
    const saved: Workflow = { ...workflow("wf-1"), description: "old" };
    const current: Workflow = { ...workflow("wf-1"), description: "new" };

    expect(hasNewerWorkflowMetadata(current, saved)).toBe(true);
  });

  it("treats an empty agent_profile_id as unchanged from an absent saved agent_profile_id", () => {
    // Same omitempty class of bug as description: clearing a profile override
    // produces agent_profile_id === "" locally, but the save response omits
    // the empty field, so saved.agent_profile_id is undefined.
    const saved: Workflow = { ...workflow("wf-1"), agent_profile_id: undefined };
    const current: Workflow = { ...workflow("wf-1"), agent_profile_id: toAgentProfileId("") };

    expect(hasNewerWorkflowMetadata(current, saved)).toBe(false);
  });

  it("still detects a real agent_profile_id change", () => {
    const saved: Workflow = {
      ...workflow("wf-1"),
      agent_profile_id: toAgentProfileId("profile-a"),
    };
    const current: Workflow = {
      ...workflow("wf-1"),
      agent_profile_id: toAgentProfileId("profile-b"),
    };

    expect(hasNewerWorkflowMetadata(current, saved)).toBe(true);
  });
});
