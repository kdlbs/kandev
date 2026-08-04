import { describe, expect, it } from "vitest";
import {
  WORKFLOW_INSTRUCTIONS_HEADING,
  splitWorkflowInstructions,
} from "./workflow-instructions";

describe("splitWorkflowInstructions", () => {
  it("returns the full message when no workflow heading is present", () => {
    const content = "Commit the changes.";
    expect(splitWorkflowInstructions(content)).toEqual({
      instructions: "",
      rest: content,
      hasInstructions: false,
    });
  });

  it("splits a leading workflow instructions block from the step body", () => {
    const content = `${WORKFLOW_INSTRUCTIONS_HEADING}\n\nAlways open a draft PR.\n\nCommit the changes.`;
    expect(splitWorkflowInstructions(content)).toEqual({
      instructions: "Always open a draft PR.",
      rest: "Commit the changes.",
      hasInstructions: true,
    });
  });

  it("keeps multi-line instruction bodies intact", () => {
    const content = `${WORKFLOW_INSTRUCTIONS_HEADING}\n\nLine one\nLine two\n\nStep body`;
    expect(splitWorkflowInstructions(content)).toEqual({
      instructions: "Line one\nLine two",
      rest: "Step body",
      hasInstructions: true,
    });
  });

  it("handles a workflow block with no following step body", () => {
    const content = `${WORKFLOW_INSTRUCTIONS_HEADING}\n\nOnly workflow rules.`;
    expect(splitWorkflowInstructions(content)).toEqual({
      instructions: "Only workflow rules.",
      rest: "",
      hasInstructions: true,
    });
  });

  it("does not treat a mid-message heading as workflow instructions", () => {
    const content = `Intro\n\n${WORKFLOW_INSTRUCTIONS_HEADING}\n\nNope`;
    expect(splitWorkflowInstructions(content).hasInstructions).toBe(false);
    expect(splitWorkflowInstructions(content).rest).toBe(content);
  });
});
