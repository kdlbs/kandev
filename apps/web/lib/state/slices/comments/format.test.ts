import { describe, it, expect } from "vitest";
import {
  formatPlanCommentsAsMarkdown,
  formatReviewCommentsAsMarkdown,
  formatPRFeedbackAsMarkdown,
  formatCommentsForMessage,
} from "./format";
import type { PlanComment, DiffComment, PRFeedbackComment } from "./types";

function makePlanComment(overrides: Partial<PlanComment> = {}): PlanComment {
  return {
    id: "plan-1",
    sessionId: "sess-1",
    source: "plan",
    text: "fix this",
    selectedText: "some code",
    createdAt: new Date().toISOString(),
    status: "pending",
    ...overrides,
  };
}

describe("formatPlanCommentsAsMarkdown", () => {
  it("returns empty string for empty input", () => {
    expect(formatPlanCommentsAsMarkdown([])).toBe("");
    expect(formatPlanCommentsAsMarkdown(null as unknown as PlanComment[])).toBe("");
    expect(formatPlanCommentsAsMarkdown(undefined as unknown as PlanComment[])).toBe("");
  });

  it("formats comment with selected text", () => {
    const comments = [makePlanComment({ selectedText: "const x = 1;", text: "rename variable" })];
    const result = formatPlanCommentsAsMarkdown(comments);

    expect(result).toContain("### Plan Comments");
    expect(result).toContain("```\nconst x = 1;\n```");
    expect(result).toContain("> rename variable");
    expect(result).toContain("---");
  });

  it("formats comment without selected text", () => {
    const comments = [makePlanComment({ selectedText: "", text: "general feedback" })];
    const result = formatPlanCommentsAsMarkdown(comments);

    expect(result).toContain("### Plan Comments");
    expect(result).not.toContain("```\n\n```"); // No empty code block
    expect(result).toContain("> general feedback");
  });

  it("formats multiple comments", () => {
    const comments = [
      makePlanComment({ selectedText: "code1", text: "comment1" }),
      makePlanComment({ id: "plan-2", selectedText: "code2", text: "comment2" }),
    ];
    const result = formatPlanCommentsAsMarkdown(comments);

    expect(result).toContain("```\ncode1\n```");
    expect(result).toContain("> comment1");
    expect(result).toContain("```\ncode2\n```");
    expect(result).toContain("> comment2");
  });

  it("handles multiline comment text with blockquotes", () => {
    const comments = [makePlanComment({ text: "line one\nline two\nline three" })];
    const result = formatPlanCommentsAsMarkdown(comments);

    expect(result).toContain("> line one\n> line two\n> line three");
  });

  it("includes header and separator", () => {
    const comments = [makePlanComment()];
    const result = formatPlanCommentsAsMarkdown(comments);

    expect(result.startsWith("### Plan Comments\n")).toBe(true);
    expect(result).toContain("\n---\n");
  });
});

describe("formatReviewCommentsAsMarkdown", () => {
  function makeDiffComment(overrides: Partial<DiffComment> = {}): DiffComment {
    return {
      id: "diff-1",
      sessionId: "sess-1",
      source: "diff",
      filePath: "src/app.ts",
      startLine: 10,
      endLine: 12,
      side: "additions",
      codeContent: "const x = 1;",
      text: "fix this",
      createdAt: new Date().toISOString(),
      status: "pending",
      ...overrides,
    };
  }

  it("returns empty string for empty input", () => {
    expect(formatReviewCommentsAsMarkdown([])).toBe("");
  });

  it("formats single comment", () => {
    const result = formatReviewCommentsAsMarkdown([makeDiffComment()]);

    expect(result).toContain("### Review Comments");
    expect(result).toContain("**src/app.ts:10-12**");
    expect(result).toContain("```\nconst x = 1;\n```");
    expect(result).toContain("> fix this");
  });

  it("formats same-line range", () => {
    const result = formatReviewCommentsAsMarkdown([makeDiffComment({ startLine: 5, endLine: 5 })]);
    expect(result).toContain("**src/app.ts:5**");
  });
});

describe("formatPRFeedbackAsMarkdown", () => {
  function makePRFeedback(overrides: Partial<PRFeedbackComment> = {}): PRFeedbackComment {
    return {
      id: "pr-1",
      sessionId: "sess-1",
      source: "pr-feedback",
      text: "",
      prNumber: 123,
      feedbackType: "review",
      content: "Please fix the failing tests",
      createdAt: new Date().toISOString(),
      status: "pending",
      ...overrides,
    };
  }

  it("returns empty string for empty input", () => {
    expect(formatPRFeedbackAsMarkdown([])).toBe("");
  });

  it("formats PR feedback", () => {
    const result = formatPRFeedbackAsMarkdown([makePRFeedback()]);

    expect(result).toContain("### PR Feedback");
    expect(result).toContain("Please fix the failing tests");
    expect(result).toContain("---");
  });
});

describe("formatCommentsForMessage", () => {
  it("separates comments by type", () => {
    const diff: DiffComment = {
      id: "d1",
      sessionId: "s",
      source: "diff",
      filePath: "f.ts",
      startLine: 1,
      endLine: 1,
      side: "additions",
      codeContent: "code",
      text: "fix",
      createdAt: "",
      status: "pending",
    };
    const plan: PlanComment = {
      id: "p1",
      sessionId: "s",
      source: "plan",
      selectedText: "text",
      text: "comment",
      createdAt: "",
      status: "pending",
    };

    const result = formatCommentsForMessage([diff, plan]);

    expect(result.diffComments).toHaveLength(1);
    expect(result.planComments).toHaveLength(1);
    expect(result.prFeedbackComments).toHaveLength(0);
  });
});
