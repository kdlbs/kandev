import { describe, expect, it } from "vitest";
import {
  MOVE_INSTRUCTIONS_END,
  MOVE_INSTRUCTIONS_HEADING,
  WORKFLOW_INSTRUCTIONS_END,
  WORKFLOW_INSTRUCTIONS_HEADING,
  splitMessageSegments,
  splitMoveInstructions,
  splitWorkflowInstructions,
} from "./workflow-instructions";

function block(body: string, rest = ""): string {
  const head = WORKFLOW_INSTRUCTIONS_HEADING + "\n\n" + body + "\n\n" + WORKFLOW_INSTRUCTIONS_END;
  return rest ? head + "\n\n" + rest : head;
}

function moveBlock(body: string): string {
  return MOVE_INSTRUCTIONS_HEADING + "\n\n" + body + "\n\n" + MOVE_INSTRUCTIONS_END;
}

const COMMIT = "Commit the changes.";
const DRAFT_PR = "Always open a draft PR.";
const DO_WORK = "Do the work.";
const FOCUS_TESTS = "Focus on tests only.";

describe("splitWorkflowInstructions", () => {
  it("returns the full message when no workflow heading is present", () => {
    const content = COMMIT;
    expect(splitWorkflowInstructions(content)).toEqual({
      instructions: "",
      rest: content,
      hasInstructions: false,
    });
  });

  it("splits a leading workflow instructions block from the step body", () => {
    expect(splitWorkflowInstructions(block(DRAFT_PR, COMMIT))).toEqual({
      instructions: DRAFT_PR,
      rest: COMMIT,
      hasInstructions: true,
    });
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

describe("splitMoveInstructions", () => {
  it("returns the full message when no move heading is present", () => {
    const content = "Commit the changes.";
    expect(splitMoveInstructions(content)).toEqual({
      before: content,
      instructions: "",
      after: "",
      hasInstructions: false,
    });
  });

  it("splits a trailing move instructions block from the preceding step body", () => {
    const content = DO_WORK + "\n\n" + moveBlock(FOCUS_TESTS);
    expect(splitMoveInstructions(content)).toEqual({
      before: DO_WORK,
      instructions: FOCUS_TESTS,
      after: "",
      hasInstructions: true,
    });
  });

  it("does not treat a heading without its end marker as a move block", () => {
    const content = "Intro\n\n" + MOVE_INSTRUCTIONS_HEADING + "\n\nUser wrote this.";
    expect(splitMoveInstructions(content).hasInstructions).toBe(false);
    expect(splitMoveInstructions(content).before).toBe(content);
  });

  it("does not treat a heading-prefix line as a move block", () => {
    const content = MOVE_INSTRUCTIONS_HEADING + " for release\n\n" + MOVE_INSTRUCTIONS_END;
    expect(splitMoveInstructions(content).hasInstructions).toBe(false);
  });
});

describe("splitMessageSegments", () => {
  it("returns a single text segment for a plain message", () => {
    expect(splitMessageSegments(COMMIT)).toEqual([{ type: "text", content: COMMIT }]);
  });

  it("returns an empty list for empty content", () => {
    expect(splitMessageSegments("")).toEqual([]);
  });

  it("collapses a leading durable block ahead of the step body", () => {
    expect(splitMessageSegments(block(DRAFT_PR, COMMIT))).toEqual([
      { type: "instructions", kind: "workflow", content: DRAFT_PR },
      { type: "text", content: COMMIT },
    ]);
  });

  it("collapses a trailing one-time move block after the step body", () => {
    const content = DO_WORK + "\n\n" + moveBlock(FOCUS_TESTS);
    expect(splitMessageSegments(content)).toEqual([
      { type: "text", content: DO_WORK },
      { type: "instructions", kind: "move", content: FOCUS_TESTS },
    ]);
  });

  it("collapses both durable and move blocks around the step body in order", () => {
    const content = block(DRAFT_PR, DO_WORK) + "\n\n" + moveBlock("Reset first.");
    expect(splitMessageSegments(content)).toEqual([
      { type: "instructions", kind: "workflow", content: DRAFT_PR },
      { type: "text", content: DO_WORK },
      { type: "instructions", kind: "move", content: "Reset first." },
    ]);
  });
});
