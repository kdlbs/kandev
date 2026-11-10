import { describe, expect, it } from "vitest";
import type { AgentMessageComment } from "@/lib/state/slices/comments";
import { composeCommentBody } from "./use-thread-extracts";

const extract = (authorName: string, quote: string, text: string): AgentMessageComment => ({
  id: quote,
  sessionId: "office-task:t",
  source: "agent-message",
  messageId: "c",
  authorName,
  selectedText: quote,
  anchor: {
    messageId: "c",
    start: 0,
    end: quote.length,
    selectedText: quote,
    prefix: "",
    suffix: "",
  },
  text,
  createdAt: "",
  status: "pending",
});

describe("composeCommentBody", () => {
  it("returns the trimmed text when there are no extracts", () => {
    expect(composeCommentBody([], "  hello ")).toBe("hello");
  });

  it("puts the extracts block before the user's text", () => {
    const body = composeCommentBody([extract("CEO", "the plan", "Too vague.")], "Please revise.");
    expect(body.startsWith("### Agent Message Comments")).toBe(true);
    expect(body).toContain("Selected comment from CEO:\n> the plan");
    expect(body.endsWith("---\n\nPlease revise.")).toBe(true);
  });

  it("posts the block alone when the text is empty", () => {
    const body = composeCommentBody([extract("CEO", "x", "y")], "  ");
    expect(body.endsWith("---")).toBe(true);
  });
});
