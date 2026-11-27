import { describe, expect, it } from "vitest";
import { workflowId as toWorkflowId } from "@/lib/types/ids";
import { stepUpdatePayload } from "./workflow-step-equality";

describe("workflow step update payload", () => {
  it("preserves true and false workflow fallback veto values", () => {
    const enabled = Object.assign(
      {
        id: "step-1",
        workflow_id: toWorkflowId("wf-1"),
        name: "Review",
        position: 1,
        color: "blue",
        created_at: "",
        updated_at: "",
      },
      { disable_unclassified_fallback: true },
    );
    const disabled = Object.assign({}, enabled, { disable_unclassified_fallback: false });

    expect(stepUpdatePayload(enabled)).toMatchObject({ disable_unclassified_fallback: true });
    expect(stepUpdatePayload(disabled)).toMatchObject({ disable_unclassified_fallback: false });
  });
});
