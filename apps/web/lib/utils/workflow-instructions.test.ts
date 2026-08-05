import { describe, expect, it } from "vitest";
import {
  WORKFLOW_INSTRUCTIONS_END,
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
    const content = `${WORKFLOW_INSTRUCTIONS_HEADING}\n\nAlways open a draft PR.\n\n${WORKFLOW_INSTRUCTIONS_END}\n\nCommit the changes.`;
    expect(splitWorkflowInstructions(content)).toEqual({
      instructions: "Always open a draft PR.",
      rest: "Commit the changes.",
      hasInstructions: true,
    });
  });

  it("keeps multi-line and multi-paragraph instruction bodies intact", () => {
    const content = `${WORKFLOW_INSTRUCTIONS_HEADING}\n\nLine one\n\nLine two\n\n${WORKFLOW_INSTRUCTIONS_END}\n\nStep body`;
    expect(splitWorkflowInstructions(content)).toEqual({
      instructions: "Line one\n\nLine two",
      rest: "Step body",
      hasInstructions: true,
    });
  });

  it("handles a workflow block with no following step body", () => {
    const content = `${WORKFLOW_INSTRUCTIONS_HEADING}\n\nOnly workflow rules.\n\n${WORKFLOW_INSTRUCTIONS_END}`;
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

  it("falls back to first blank line for legacy messages without an end marker", () => {
    const content = `${WORKFLOW_INSTRUCTIONS_HEADING}\n\nAlways open a draft PR.\n\nCommit the changes.`;
    expect(splitWorkflowInstructions(content)).toEqual({
      instructions: "Always open a draft PR.",
      rest: "Commit the changes.",
      hasInstructions: true,
    });
  });

  it("uses the last end marker when the body quotes the token", () => {
    const body = `Never emit ${WORKFLOW_INSTRUCTIONS_END} in docs.`;
    const content = `${WORKFLOW_INSTRUCTIONS_HEADING}\n\n${body}\n\n${WORKFLOW_INSTRUCTIONS_END}\n\nStep body`;
    expect(splitWorkflowInstructions(content)).toEqual({
      instructions: body,
      rest: "Step body",
      hasInstructions: true,
    });
  });
});
