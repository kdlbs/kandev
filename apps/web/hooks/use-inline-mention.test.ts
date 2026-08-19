import { cleanup, renderHook, act } from "@testing-library/react";
import { afterEach, describe, it, expect, vi } from "vitest";

vi.mock("@/hooks/domains/settings/use-custom-prompts", () => ({
  useCustomPrompts: () => ({
    prompts: [
      { id: "builtin", name: "changes-walkthrough", content: "walkthrough" },
      { id: "qa-multi", name: "qa-multi", content: "multi-line content" },
    ],
    loaded: true,
    loading: false,
  }),
}));

import {
  makePromptItem,
  detectMentionTrigger,
  filterItems,
  useInlineMention,
  type MentionItem,
} from "./use-inline-mention";
import type { RichTextInputHandle } from "@/components/task/chat/rich-text-input";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

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

function makeMutableInput(state: { value: string; caretPos: number }): RichTextInputHandle {
  return {
    focus: vi.fn(),
    blur: vi.fn(),
    getSelectionStart: () => state.caretPos,
    getSelectionEnd: () => state.caretPos,
    setSelectionRange: (start: number) => {
      state.caretPos = start;
    },
    getCaretRect: () => ({ x: 0, y: 0 }) as DOMRect,
    getValue: () => state.value,
    setValue: vi.fn(),
    insertText: vi.fn(),
    getTextareaElement: () => null,
  };
}

describe("useInlineMention scheduling", () => {
  it("ignores a stale animation frame after a newer input change", () => {
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());

    const inputState = { value: "", caretPos: 0 };
    const inputRef = { current: makeMutableInput(inputState) };
    const { result, rerender } = renderHook(
      ({ value }: { value: string }) =>
        useInlineMention({
          inputRef,
          value,
          onChange: (nextValue) => {
            inputState.value = nextValue;
            inputState.caretPos = nextValue.length;
          },
          promptInsertMode: "inline",
        }),
      { initialProps: { value: "" } },
    );

    act(() => {
      inputState.value = "@";
      inputState.caretPos = 1;
      result.current.handleChange("@", 1);
      rerender({ value: "@" });
    });
    act(() => {
      inputState.value = "@qa-mu";
      inputState.caretPos = 6;
      result.current.handleChange("@qa-mu", 6);
      rerender({ value: "@qa-mu" });
    });

    expect(frames).toHaveLength(2);
    inputState.caretPos = 1;
    act(() => {
      frames[1](0);
      frames[0](0);
    });

    expect(result.current.query).toBe("qa-mu");
    expect(result.current.items.map((item) => item.label)).toEqual(["qa-multi"]);
  });
});

describe("useInlineMention Escape", () => {
  it("claims the key and restores input focus after closing the menu", () => {
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());

    const inputState = { value: "@qa", caretPos: 3 };
    const input = makeMutableInput(inputState);
    const inputRef = { current: input };
    const { result } = renderHook(() =>
      useInlineMention({
        inputRef,
        value: inputState.value,
        onChange: (nextValue) => {
          inputState.value = nextValue;
        },
        promptInsertMode: "inline",
      }),
    );

    act(() => result.current.handleChange("@qa", 3));
    act(() => frames.shift()?.(0));
    expect(result.current.isOpen).toBe(true);

    const preventDefault = vi.fn();
    const stopPropagation = vi.fn();
    act(() => {
      result.current.handleKeyDown({
        key: "Escape",
        preventDefault,
        stopPropagation,
      } as unknown as React.KeyboardEvent);
    });

    expect(preventDefault).toHaveBeenCalledOnce();
    expect(stopPropagation).toHaveBeenCalledOnce();
    expect(result.current.isOpen).toBe(false);
    expect(frames).toHaveLength(1);

    act(() => frames.shift()?.(0));
    expect(input.focus).toHaveBeenCalledOnce();
  });
});

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
