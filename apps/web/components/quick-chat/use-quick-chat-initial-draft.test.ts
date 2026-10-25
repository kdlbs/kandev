import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";
import { useQuickChatInitialDraft } from "./use-quick-chat-initial-draft";

const DRAFT = "why is KAN-418 here?";

function createHandle(getValue = vi.fn().mockReturnValue("")) {
  return {
    focusInput: vi.fn(),
    getTextareaElement: vi.fn().mockReturnValue(document.createElement("div")),
    getValue,
    getSelectionStart: vi.fn().mockReturnValue(0),
    insertText: vi.fn(),
    clear: vi.fn(),
    getAttachments: vi.fn().mockReturnValue([]),
  } as unknown as ChatInputContainerHandle;
}

describe("useQuickChatInitialDraft", () => {
  it("applies a pending draft once the composer ref is available", () => {
    const handle = createHandle();
    const chatInputRef = { current: handle };

    renderHook(() => useQuickChatInitialDraft({ draft: DRAFT, chatInputRef }));

    expect(handle.clear).toHaveBeenCalledTimes(1);
    expect(handle.insertText).toHaveBeenCalledWith(DRAFT, 0, 0);
    expect(handle.focusInput).toHaveBeenCalledTimes(1);
  });

  it("holds a draft until the composer mounts, then applies it", () => {
    const chatInputRef: { current: ChatInputContainerHandle | null } = { current: null };
    const { rerender } = renderHook(() => useQuickChatInitialDraft({ draft: DRAFT, chatInputRef }));

    const handle = createHandle();
    chatInputRef.current = handle;
    rerender();

    expect(handle.insertText).toHaveBeenCalledWith(DRAFT, 0, 0);
  });

  it("does not re-apply the same draft value on a later render", () => {
    const handle = createHandle();
    const chatInputRef = { current: handle };
    const { rerender } = renderHook(
      ({ draft }) => useQuickChatInitialDraft({ draft, chatInputRef }),
      { initialProps: { draft: "same draft" } },
    );
    rerender({ draft: "same draft" });

    expect(handle.insertText).toHaveBeenCalledTimes(1);
  });

  it("re-applies an identical draft after the caller passes undefined first", () => {
    const handle = createHandle();
    const chatInputRef = { current: handle };
    const { rerender } = renderHook<void, { draft: string | undefined }>(
      ({ draft }) => useQuickChatInitialDraft({ draft, chatInputRef }),
      {
        initialProps: { draft: "repeat me" },
      },
    );

    rerender({ draft: undefined });
    rerender({ draft: "repeat me" });

    expect(handle.insertText).toHaveBeenCalledTimes(2);
  });

  it("leaves the composer untouched when the draft is undefined", () => {
    const handle = createHandle();
    const chatInputRef = { current: handle };

    renderHook(() => useQuickChatInitialDraft({ draft: undefined, chatInputRef }));

    expect(handle.clear).not.toHaveBeenCalled();
    expect(handle.insertText).not.toHaveBeenCalled();
  });

  it("leaves the composer untouched when the draft is an empty string", () => {
    const handle = createHandle();
    const chatInputRef = { current: handle };

    renderHook(() => useQuickChatInitialDraft({ draft: "", chatInputRef }));

    expect(handle.clear).not.toHaveBeenCalled();
    expect(handle.insertText).not.toHaveBeenCalled();
  });

  it("holds a draft while the composer ref exists but its editor is not ready yet, then applies it once ready", () => {
    const getTextareaElement = vi.fn().mockReturnValue(null);
    const handle = { ...createHandle(), getTextareaElement };
    const chatInputRef = { current: handle };

    const { rerender } = renderHook(() => useQuickChatInitialDraft({ draft: DRAFT, chatInputRef }));

    expect(handle.clear).not.toHaveBeenCalled();
    expect(handle.insertText).not.toHaveBeenCalled();

    getTextareaElement.mockReturnValue(document.createElement("div"));
    rerender();

    expect(handle.insertText).toHaveBeenCalledWith(DRAFT, 0, 0);
    expect(handle.focusInput).toHaveBeenCalledTimes(1);
  });

  it("keeps the draft pending while initialPrompt is non-empty, then applies it once the prompt clears", () => {
    const handle = createHandle();
    const chatInputRef = { current: handle };
    const { rerender } = renderHook(
      ({ initialPrompt }) =>
        useQuickChatInitialDraft({ draft: "pending draft", initialPrompt, chatInputRef }),
      { initialProps: { initialPrompt: "Start here" as string | undefined } },
    );

    expect(handle.insertText).not.toHaveBeenCalled();

    rerender({ initialPrompt: undefined });

    expect(handle.insertText).toHaveBeenCalledWith("pending draft", 0, 0);
  });
});
