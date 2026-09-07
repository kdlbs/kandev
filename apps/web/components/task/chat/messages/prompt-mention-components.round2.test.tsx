import { describe, expect, it } from "vitest";
import { splitMarkdownPromptMentionSegments } from "./prompt-mention-components";

const promptNames = ["daily"];

function promptValues(content: string) {
  return splitMarkdownPromptMentionSegments(content, promptNames)
    .filter((segment) => segment.kind === "prompt")
    .map((segment) => segment.value);
}

describe("splitMarkdownPromptMentionSegments round-two boundaries", () => {
  it("does not reinterpret a mismatched backtick run as a delimiter", () => {
    expect(promptValues("`foo`` @daily`")).toEqual([]);
    expect(promptValues("``` @daily")).toEqual([]);
  });

  it("treats backslashes literally while scanning an inline code span", () => {
    expect(promptValues("`foo @daily\\` tail")).toEqual([]);
  });

  it("recognizes aliases after a CRLF closing fence", () => {
    expect(promptValues("```\r\ncode\r\n```\r\n@daily")).toEqual(["@daily"]);
  });

  it("does not use a code-span bracket as an inline-link label", () => {
    expect(promptValues('`[`](/url "title @daily")')).toEqual(["@daily"]);
  });
  it("does not hide a title alias after reference-style link syntax", () => {
    expect(promptValues('[label][ref](url "title @daily")')).toEqual(["@daily"]);
  });

  it("rejects escaped whitespace in a bare link destination", () => {
    expect(promptValues('[label](url\\ bar "title @daily")')).toEqual(["@daily"]);
  });

  it("skips titles in adjacent valid inline links", () => {
    expect(promptValues('[] [label](url "title @daily")')).toEqual([]);
  });

  it("keeps formatted aliases adjacent to ordinary text as text", () => {
    expect(promptValues("x**@daily**")).toEqual([]);
    expect(promptValues("x_@daily_")).toEqual([]);
  });

  it("keeps the longest prompt name when names share a prefix", () => {
    expect(
      splitMarkdownPromptMentionSegments("@Daily Summary", ["Daily", "Daily Summary"]),
    ).toEqual([{ kind: "prompt", value: "@Daily Summary", name: "Daily Summary" }]);
  });

  it("recognizes aliases nested in Markdown formatting", () => {
    expect(promptValues("**_@daily_**")).toEqual(["@daily"]);
    expect(promptValues("[**@daily**](/url)")).toEqual(["@daily"]);
  });

  it("does not hide aliases after malformed link-like text", () => {
    expect(promptValues('[x]foo "title @daily")')).toEqual(["@daily"]);
  });

  it("recognizes aliases after CR-only fenced code", () => {
    expect(promptValues("~~~\rcode\r~~~\r@daily")).toEqual(["@daily"]);
  });

  it("recognizes aliases in link labels adjacent to ordinary text", () => {
    expect(promptValues("x[@daily](url)")).toEqual(["@daily"]);
  });

  it("recognizes aliases at the start of formatted text", () => {
    expect(promptValues("**@daily now**")).toEqual(["@daily"]);
    expect(promptValues("[@daily label](/url)")).toEqual(["@daily"]);
  });

  it("does not close formatted aliases on escaped delimiters", () => {
    expect(promptValues("**@daily\\*")).toEqual([]);
    expect(promptValues("[@daily\\]")).toEqual([]);
  });
  it("recognizes aliases in supported long messages", () => {
    const content = `${"x".repeat(32_769)} @daily`;

    expect(splitMarkdownPromptMentionSegments(content, promptNames)).toEqual([
      { kind: "text", value: `${"x".repeat(32_769)} ` },
      { kind: "prompt", value: "@daily", name: "daily" },
    ]);
  });

  it("fails closed for oversized malformed Markdown mention input", () => {
    const content = `[${"x".repeat(40_000)} [@daily`;

    expect(splitMarkdownPromptMentionSegments(content, promptNames)).toEqual([
      { kind: "text", value: content },
    ]);
  });
});
