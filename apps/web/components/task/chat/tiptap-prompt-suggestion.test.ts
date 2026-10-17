import { afterEach, describe, expect, it } from "vitest";
import { Editor } from "@tiptap/core";
import Document from "@tiptap/extension-document";
import Paragraph from "@tiptap/extension-paragraph";
import Text from "@tiptap/extension-text";
import { DynamicPlaceholder, updateDynamicPlaceholder } from "./tiptap-dynamic-placeholder";
import {
  PromptSuggestion,
  acceptPromptSuggestion,
  fillPromptSuggestionForSubmit,
  submitWithPromptSuggestion,
  updatePromptSuggestion,
  visiblePromptSuggestion,
} from "./tiptap-prompt-suggestion";

const RUN_TESTS = "sim, corre os testes";

let editor: Editor | null = null;

function makeEditor(content = "") {
  editor = new Editor({
    element: document.createElement("div"),
    extensions: [Document, Paragraph, Text, DynamicPlaceholder, PromptSuggestion],
    content,
  });
  updateDynamicPlaceholder(editor, "Ask the agent");
  return editor;
}

function firstParagraph(e: Editor) {
  return e.view.dom.querySelector("p");
}

afterEach(() => {
  editor?.destroy();
  editor = null;
});

// @covers AC-UI-PROMPT-SUGGEST-004.1 AC-UI-PROMPT-SUGGEST-004.5
describe("prompt suggestion ghost text", () => {
  it("renders the suggestion in place of the placeholder while the draft is empty", () => {
    const e = makeEditor();
    updatePromptSuggestion(e, RUN_TESTS);
    expect(firstParagraph(e)?.getAttribute("data-prompt-suggestion")).toBe(RUN_TESTS);
    expect(firstParagraph(e)?.getAttribute("data-placeholder")).toBe("Ask the agent");
    expect(firstParagraph(e)?.classList.contains("has-prompt-suggestion")).toBe(true);
    expect(visiblePromptSuggestion(e)).toBe(RUN_TESTS);
  });

  it("keeps the normal placeholder without a suggestion", () => {
    const e = makeEditor();
    updatePromptSuggestion(e, null);
    expect(firstParagraph(e)?.getAttribute("data-placeholder")).toBe("Ask the agent");
    expect(firstParagraph(e)?.hasAttribute("data-prompt-suggestion")).toBe(false);
    expect(firstParagraph(e)?.classList.contains("has-prompt-suggestion")).toBe(false);
  });

  it("is not visible once the draft has content", () => {
    const e = makeEditor("<p>typed</p>");
    updatePromptSuggestion(e, "sim");
    expect(visiblePromptSuggestion(e)).toBeNull();
    expect(acceptPromptSuggestion(e)).toBe(false);
    expect(e.getText()).toBe("typed");
  });
});

// @covers AC-UI-PROMPT-SUGGEST-004.2 AC-UI-PROMPT-SUGGEST-004.3
describe("accepting a suggestion", () => {
  it("fills the draft as editable text", () => {
    const e = makeEditor();
    updatePromptSuggestion(e, "sim, corre");
    expect(acceptPromptSuggestion(e)).toBe(true);
    expect(e.getText()).toBe("sim, corre");
    expect(visiblePromptSuggestion(e)).toBeNull();
  });

  it("fills the draft before submit only when the suggestion is visible", () => {
    const e = makeEditor();
    expect(fillPromptSuggestionForSubmit(e)).toBe(false);
    updatePromptSuggestion(e, "sim, faz commit");
    expect(fillPromptSuggestionForSubmit(e)).toBe(true);
    expect(e.getText()).toBe("sim, faz commit");
  });

  // @covers AC-UI-PROMPT-SUGGEST-004.3
  it("submits the visible suggestion, so the send button and the shortcut agree", () => {
    const e = makeEditor();
    updatePromptSuggestion(e, RUN_TESTS);
    const sent: string[] = [];
    submitWithPromptSuggestion(e, () => sent.push(e.getText()));
    expect(sent).toEqual([RUN_TESTS]);
  });

  it("submits a typed draft unchanged", () => {
    const e = makeEditor("o meu texto");
    updatePromptSuggestion(e, "sim");
    const sent: string[] = [];
    submitWithPromptSuggestion(e, () => sent.push(e.getText()));
    expect(sent).toEqual(["o meu texto"]);
  });

  it("does nothing while the editor is not editable", () => {
    const e = makeEditor();
    updatePromptSuggestion(e, "sim");
    e.setEditable(false);
    expect(acceptPromptSuggestion(e)).toBe(false);
    expect(fillPromptSuggestionForSubmit(e)).toBe(false);
  });
});
