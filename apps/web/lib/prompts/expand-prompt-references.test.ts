import { describe, expect, it } from "vitest";
import {
  collectPromptReferenceExpansions,
  formatPromptReferenceExpansions,
  type PromptReference,
} from "./expand-prompt-references";

function prompt(name: string, content: string): PromptReference {
  return { id: name, name, content };
}

describe("collectPromptReferenceExpansions", () => {
  it("recursively collects saved prompt references without rewriting content", () => {
    const prompts = [
      prompt("outer", "Before @middle after"),
      prompt("middle", "middle says @inner"),
      prompt("inner", "resolved inner"),
    ];

    expect(collectPromptReferenceExpansions("@outer", prompts)).toEqual([
      { name: "outer", content: "Before @middle after" },
      { name: "middle", content: "middle says @inner" },
      { name: "inner", content: "resolved inner" },
    ]);
  });

  it("leaves unknown references and inline email-like text unchanged", () => {
    const prompts = [prompt("known", "resolved")];

    expect(collectPromptReferenceExpansions("ping a@known @missing @known.", prompts)).toEqual([
      { name: "known", content: "resolved" },
    ]);
  });

  it("does not recurse forever when prompts reference each other", () => {
    const prompts = [prompt("outer", "Outer @inner"), prompt("inner", "Inner @outer")];

    expect(collectPromptReferenceExpansions("@outer", prompts)).toEqual([
      { name: "outer", content: "Outer @inner" },
      { name: "inner", content: "Inner @outer" },
    ]);
    expect(collectPromptReferenceExpansions("@inner", prompts, "outer")).toEqual([
      { name: "inner", content: "Inner @outer" },
    ]);
  });
});

describe("formatPromptReferenceExpansions", () => {
  it("renders expansions as supplemental kandev-system block content", () => {
    const out = formatPromptReferenceExpansions([
      { name: "improve-harness", content: "Review durable harness improvements." },
    ]);

    expect(out).toContain("EXPANDED PROMPT REFERENCES");
    expect(out).toContain("### @improve-harness");
    expect(out).toContain("Review durable harness improvements.");
  });
});
