import { describe, expect, it } from "vitest";
import { computeSingleWorkflowFallbackId } from "./task-create-dialog-defaults";

describe("computeSingleWorkflowFallbackId", () => {
  it("selects the sole visible workflow when hidden workflows are also loaded", () => {
    const workflowId = computeSingleWorkflowFallbackId(null, null, [
      { id: "kanban", hidden: false },
      { id: "improve-kandev", hidden: true },
    ]);

    expect(workflowId).toBe("kanban");
  });
});
