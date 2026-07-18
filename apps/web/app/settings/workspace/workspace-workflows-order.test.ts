import { describe, expect, it } from "vitest";
import { workflowId as toWorkflowId, type Workflow } from "@/lib/types/http";
import {
  alignSavedWorkflowsToDraftOrder,
  getWorkflowOrderDirtyIds,
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
