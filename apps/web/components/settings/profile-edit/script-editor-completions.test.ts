import { describe, expect, it } from "vitest";
import { createPromptMentionCompletionProvider } from "./script-editor-completions";
import type { PromptReference } from "@/lib/prompts/expand-prompt-references";
import type { editor, languages, Position } from "monaco-editor";

const monacoStub = {
  languages: {
    CompletionItemKind: { Reference: 17 },
  },
} as unknown as typeof import("monaco-editor");

function modelForLine(line: string): editor.ITextModel {
  return {
    getLineContent: () => line,
  } as unknown as editor.ITextModel;
}

function complete(
  prompts: PromptReference[],
  line: string,
  column: number,
): languages.CompletionList {
  const provider = createPromptMentionCompletionProvider(monacoStub, prompts);
  const result = provider.provideCompletionItems(
    modelForLine(line),
    { lineNumber: 1, column } as unknown as Position,
    {} as languages.CompletionContext,
    {} as import("monaco-editor").CancellationToken,
  );
  return result as languages.CompletionList;
}

const prompts: PromptReference[] = [
  { id: "1", name: "review", content: "Review this code carefully for bugs." },
  { id: "2", name: "release-notes", content: "Write release notes." },
];

describe("createPromptMentionCompletionProvider", () => {
  it("has @ as its only trigger character", () => {
    const provider = createPromptMentionCompletionProvider(monacoStub, prompts);
    expect(provider.triggerCharacters).toEqual(["@"]);
  });

  it("suggests prompts when @ is at the start of a line", () => {
    const { suggestions } = complete(prompts, "@", 2);
    expect(suggestions.map((s) => (s.label as { label: string }).label ?? s.label)).toEqual([
      "@review",
      "@release-notes",
    ]);
  });

  it("suggests prompts when @ follows whitespace", () => {
    const { suggestions } = complete(prompts, "please @re", 11);
    expect(suggestions).toHaveLength(2);
    expect(suggestions[0].insertText).toBe("review");
  });

  it("does not suggest when @ is mid-word", () => {
    const { suggestions } = complete(prompts, "foo@bar", 8);
    expect(suggestions).toHaveLength(0);
  });

  it("does not suggest when there is no @ before the cursor", () => {
    const { suggestions } = complete(prompts, "hello", 6);
    expect(suggestions).toHaveLength(0);
  });

  it("returns no suggestions when prompts list is empty", () => {
    const { suggestions } = complete([], "@", 2);
    expect(suggestions).toHaveLength(0);
  });

  it("includes a content preview in detail/documentation", () => {
    const { suggestions } = complete(prompts, "@", 2);
    expect(suggestions[0].detail).toBe("Review this code carefully for bugs.");
  });
});
