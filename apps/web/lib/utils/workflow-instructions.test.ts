import { describe, expect, it } from "vitest";
import {
  WORKFLOW_INSTRUCTIONS_END,
  WORKFLOW_INSTRUCTIONS_HEADING,
  splitWorkflowInstructions,
} from "./workflow-instructions";

function block(body: string, rest = ""): string {
  const head = WORKFLOW_INSTRUCTIONS_HEADING + "\n\n" + body + "\n\n" + WORKFLOW_INSTRUCTIONS_END;
  return rest ? head + "\n\n" + rest : head;
}

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
    expect(splitWorkflowInstructions(block("Always open a draft PR.", "Commit the changes."))).toEqual(
      {
        instructions: "Always open a draft PR.",
        rest: "Commit the changes.",
        hasInstructions: true,
      },
    );
  });

  it("keeps multi-line and multi-paragraph instruction bodies intact", () => {
    expect(splitWorkflowInstructions(block("Line one\n\nLine two", "Step body"))).toEqual({
      instructions: "Line one\n\nLine two",
      rest: "Step body",
      hasInstructions: true,
    });
  });

  it("handles a workflow block with no following step body", () => {
    expect(splitWorkflowInstructions(block("Only workflow rules."))).toEqual({
      instructions: "Only workflow rules.",
      rest: "",
      hasInstructions: true,
    });
  });

  it("does not treat a mid-message heading as workflow instructions", () => {
    const content = "Intro\n\n" + WORKFLOW_INSTRUCTIONS_HEADING + "\n\nNope";
    expect(splitWorkflowInstructions(content).hasInstructions).toBe(false);
    expect(splitWorkflowInstructions(content).rest).toBe(content);
  });

  it("does not treat a heading-prefix line as workflow instructions", () => {
    const content = WORKFLOW_INSTRUCTIONS_HEADING + " for release\n\nbody";
    expect(splitWorkflowInstructions(content)).toEqual({
      instructions: "",
      rest: content,
      hasInstructions: false,
    });
  });

  it("ignores marker-less user content that only starts with the heading", () => {
    const content = WORKFLOW_INSTRUCTIONS_HEADING + "\n\nUser wrote this.\n\nMore text.";
    expect(splitWorkflowInstructions(content)).toEqual({
      instructions: "",
      rest: content,
      hasInstructions: false,
    });
  });

  it("uses the first standalone end marker, not a quoted copy in the rest", () => {
    const rest = "See docs mentioning " + WORKFLOW_INSTRUCTIONS_END + " later.\n\nDo work.";
    expect(splitWorkflowInstructions(block("Rule one.", rest))).toEqual({
      instructions: "Rule one.",
      rest,
      hasInstructions: true,
    });
  });

  it("keeps body text that quotes the end marker when the structural marker follows", () => {
    const body = "Never emit " + WORKFLOW_INSTRUCTIONS_END + " in docs.";
    expect(splitWorkflowInstructions(block(body, "Step body"))).toEqual({
      instructions: body,
      rest: "Step body",
      hasInstructions: true,
    });
  });
});
