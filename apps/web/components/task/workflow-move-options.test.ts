import { describe, expect, it } from "vitest";
import { workflowMoveOptionsPayload } from "./workflow-move-options";

describe("workflowMoveOptionsPayload", () => {
  it("trims values and omits empty overrides", () => {
    expect(
      workflowMoveOptionsPayload({
        resetContext: true,
        instructions: "  create the PR ready for review  ",
        skipStepPrompt: true,
      }),
    ).toEqual({
      reset_context: true,
      instructions: "create the PR ready for review",
      skip_step_prompt: true,
    });
  });

  it("returns undefined for a destination-only move", () => {
    expect(
      workflowMoveOptionsPayload({
        resetContext: false,
        instructions: "",
        skipStepPrompt: false,
      }),
    ).toBeUndefined();
  });
});
