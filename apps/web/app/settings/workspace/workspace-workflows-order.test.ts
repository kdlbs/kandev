import { describe, expect, it } from "vitest";
import { workflowId as toWorkflowId, type Workflow } from "@/lib/types/http";
import { alignSavedWorkflowsToDraftOrder } from "./workspace-workflows-client";

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
      alignSavedWorkflowsToDraftOrder([draft, existing], [existing], draft.id, persisted).map(
        ({ id }) => id,
      ),
    ).toEqual([persisted.id, existing.id]);
  });
});
