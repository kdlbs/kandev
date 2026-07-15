import { describe, expect, it } from "vitest";
import { normalizeWorkflowTemplate } from "./workflow-api";

describe("normalizeWorkflowTemplate", () => {
  it("preserves template step identities used by transition references", () => {
    const template = normalizeWorkflowTemplate({
      id: "template-1",
      name: "Review flow",
      is_system: true,
      created_at: "",
      updated_at: "",
      default_steps: [
        {
          id: "in-progress",
          name: "In Progress",
          position: 0,
          events: {
            on_turn_complete: [{ type: "move_to_step", config: { step_id: "review" } }],
          },
        },
        { id: "review", name: "Review", position: 1 },
      ],
    });

    expect(template.default_steps?.map((step) => step.id)).toEqual(["in-progress", "review"]);
  });
});
