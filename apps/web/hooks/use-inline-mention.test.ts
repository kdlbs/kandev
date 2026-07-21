import { describe, it, expect, vi } from "vitest";
import {
  makePromptItem,
  detectMentionTrigger,
  filterItems,
  type MentionItem,
} from "./use-inline-mention";
import type { RichTextInputHandle } from "@/components/task/chat/rich-text-input";

function makeFakeInput(value: string, caretPos: number): RichTextInputHandle {
  let selStart = caretPos;
  let selEnd = caretPos;
  return {
    focus: vi.fn(),
    blur: vi.fn(),
    getSelectionStart: () => selStart,
    getSelectionEnd: () => selEnd,
    setSelectionRange: (start: number, end: number) => {
      selStart = start;
      selEnd = end;
    },
    getCaretRect: () => null,
    getValue: () => value,
    setValue: vi.fn(),
    insertText: vi.fn(),
    getTextareaElement: () => null,
  };
}

describe("makePromptItem — context mode (default chat behavior)", () => {
  it("deletes the @query text and calls onPromptSelect", () => {
    const prompt = { id: "p1", name: "bug-template", content: "Reproduce, isolate, fix." };
    const onPromptSelect = vi.fn();
    const item = makePromptItem(prompt, "context", onPromptSelect);

    const value = "Hello @bug";
    const triggerStart = 6;
    const cursorPos = value.length;
    const input = makeFakeInput(value, cursorPos);
    const onChange = vi.fn();

    item.onSelect(input, value, triggerStart, onChange);

    expect(onChange).toHaveBeenCalledWith("Hello ");
    expect(onPromptSelect).toHaveBeenCalledWith("p1", "bug-template");
  });

  it("exposes kind 'prompt' and label = prompt name", () => {
    const item = makePromptItem({ id: "p1", name: "foo", content: "bar" }, "context");
    expect(item.kind).toBe("prompt");
    expect(item.label).toBe("foo");
  });
});

describe("makePromptItem — inline mode (task-create behavior)", () => {
  it("replaces the @query text with the prompt content", () => {
    const prompt = {
      id: "p1",
      name: "bug-template",
      content: "Reproduce, isolate, fix with a regression test.",
    };
    const onPromptSelect = vi.fn();
    const item = makePromptItem(prompt, "inline", onPromptSelect);

    const value = "Hello @bug";
    const triggerStart = 6;
    const cursorPos = value.length;
    const input = makeFakeInput(value, cursorPos);
    const onChange = vi.fn();

    item.onSelect(input, value, triggerStart, onChange);

    expect(onChange).toHaveBeenCalledWith("Hello Reproduce, isolate, fix with a regression test.");
    expect(onPromptSelect).not.toHaveBeenCalled();
  });

  it("places caret at the end of the inserted content", async () => {
    const prompt = { id: "p1", name: "p", content: "abc" };
    const item = makePromptItem(prompt, "inline");
    const value = "x @p";
    const triggerStart = 2;
    const input = makeFakeInput(value, value.length);
    const setSelectionRangeSpy = vi.spyOn(input, "setSelectionRange");
    const focusSpy = vi.spyOn(input, "focus");
    const onChange = vi.fn();

    item.onSelect(input, value, triggerStart, onChange);

    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

    const expectedCaret = triggerStart + prompt.content.length;
    expect(setSelectionRangeSpy).toHaveBeenCalledWith(expectedCaret, expectedCaret);
    expect(focusSpy).toHaveBeenCalled();
  });

  it("preserves text after the cursor", () => {
    const prompt = { id: "p1", name: "p", content: "XYZ" };
    const item = makePromptItem(prompt, "inline");
    const value = "before @p after";
    const triggerStart = 7;
    const cursorPos = 9;
    const input = makeFakeInput(value, cursorPos);
    const onChange = vi.fn();

    item.onSelect(input, value, triggerStart, onChange);

    expect(onChange).toHaveBeenCalledWith("before XYZ after");
  });
});

describe("detectMentionTrigger", () => {
  it("returns the query when @ is at start of input", () => {
    expect(detectMentionTrigger("@foo", 4)).toEqual({ triggerStart: 0, query: "foo" });
  });

  it("returns the query when @ follows whitespace", () => {
    expect(detectMentionTrigger("hello @bar", 10)).toEqual({ triggerStart: 6, query: "bar" });
  });

  it("rejects @ inside a word (no preceding whitespace)", () => {
    expect(detectMentionTrigger("foo@bar", 7)).toBeNull();
  });

  it("rejects when whitespace appears between @ and cursor", () => {
    expect(detectMentionTrigger("@foo bar", 8)).toBeNull();
  });

  it("returns null when no @ before cursor", () => {
    expect(detectMentionTrigger("plain text", 5)).toBeNull();
  });
});

describe("filterItems — relevance ordering", () => {
  function dummyItem(id: string, label: string): MentionItem {
    return { id, kind: "prompt", label, onSelect: vi.fn() };
  }

  it("returns all items when query is empty", () => {
    const items = [dummyItem("1", "a"), dummyItem("2", "b")];
    expect(filterItems(items, "")).toHaveLength(2);
  });

  it("orders prefix matches before contains matches", () => {
    const items = [dummyItem("contains", "abc-foo"), dummyItem("prefix", "foo-bar")];
    const out = filterItems(items, "foo");
    expect(out.map((i) => i.id)).toEqual(["prefix", "contains"]);
  });

  it("returns built-in and user prompts together", () => {
    const items = [dummyItem("builtin", "review"), dummyItem("user", "review-mine")];
    const out = filterItems(items, "review");
    expect(out).toHaveLength(2);
  });
});
