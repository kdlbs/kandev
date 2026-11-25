import { act, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setChatDraftContent, setChatDraftText } from "@/lib/local-storage";
import {
  FIRST,
  expectEditor,
  flushEditor,
  mountComposer,
  pendingSend,
  savedDraft,
  settle,
} from "./chat-input-draft-ownership.test-helpers";

beforeEach(() => {
  sessionStorage.clear();
  localStorage.clear();
  vi.useFakeTimers();
});

afterEach(async () => {
  cleanup();
  await flushEditor();
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
  sessionStorage.clear();
  localStorage.clear();
});

describe("multiline editor restoration", () => {
  it("restores an accepted opening message without adding hard breaks", async () => {
    // @covers AC-TASKS-QUICK-CHAT-COMPOSER-001.9 AC-TASKS-QUICK-CHAT-COMPOSER-001.10
    const openingText = "first paragraph\n\nsecond paragraph";
    setChatDraftText(FIRST, openingText);
    setChatDraftContent(FIRST, null);
    const send = pendingSend();
    const submit = vi.fn(() => send.promise);
    const { ref } = mountComposer(FIRST, submit);

    await flushEditor();
    expect(ref.current?.value).toBe(openingText);
    expect(ref.current?.inputRef.current?.getValue()).toBe(openingText);

    const resetHeight = vi.fn();
    act(() => ref.current!.handleSubmit(resetHeight));
    expect(submit).toHaveBeenCalledExactlyOnceWith({ message: openingText });
    await settle(send);

    expectEditor(ref, "");
    expect(savedDraft(FIRST)).toEqual({ text: "", content: null, attachments: [] });
    expect(resetHeight).toHaveBeenCalledOnce();
  });

  it("preserves multiline text entered through the real editor", async () => {
    const openingText = "first paragraph\n\nsecond paragraph";
    const { ref } = mountComposer(FIRST, () => false);

    await flushEditor();
    act(() => ref.current!.inputRef.current!.setValue(openingText));

    expect(ref.current?.value).toBe(openingText);
    expect(ref.current?.inputRef.current?.getValue()).toBe(openingText);
  });
});
