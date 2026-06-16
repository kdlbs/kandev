import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type React from "react";
import { useChatInputState } from "./use-chat-input-state";
import type { TipTapInputHandle } from "./tiptap-input";

type SubmitHandler = Parameters<typeof useChatInputState>[0]["onSubmit"];

function renderInputState(onSubmit: SubmitHandler) {
  return renderHook(() =>
    useChatInputState({
      sessionId: "session-1",
      isSending: false,
      contextItems: [],
      showRequestChangesTooltip: false,
      onSubmit,
    }),
  );
}

function attachInputHandle(
  inputRef: React.RefObject<TipTapInputHandle | null>,
  clear: ReturnType<typeof vi.fn>,
) {
  (inputRef as React.MutableRefObject<Partial<TipTapInputHandle> | null>).current = {
    clear,
    getMentions: () => [],
    getTaskMentions: () => [],
  };
}

describe("useChatInputState", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("keeps the draft when async submit reports failure", async () => {
    const onSubmit = vi
      .fn<Parameters<SubmitHandler>, ReturnType<SubmitHandler>>()
      .mockResolvedValue(false);
    const clear = vi.fn();
    const { result } = renderInputState(onSubmit);

    act(() => {
      result.current.handleChange("hello");
      attachInputHandle(result.current.inputRef, clear);
    });
    await waitFor(() => expect(result.current.value).toBe("hello"));

    act(() => {
      result.current.handleSubmit(vi.fn());
    });

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith("hello", undefined, undefined, undefined, undefined),
    );
    expect(result.current.value).toBe("hello");
    expect(clear).not.toHaveBeenCalled();
  });

  it("clears the draft when async submit succeeds", async () => {
    const onSubmit = vi
      .fn<Parameters<SubmitHandler>, ReturnType<SubmitHandler>>()
      .mockResolvedValue(true);
    const clear = vi.fn();
    const resetHeight = vi.fn();
    const { result } = renderInputState(onSubmit);

    act(() => {
      result.current.handleChange("hello");
      attachInputHandle(result.current.inputRef, clear);
    });
    await waitFor(() => expect(result.current.value).toBe("hello"));

    act(() => {
      result.current.handleSubmit(resetHeight);
    });

    await waitFor(() => expect(result.current.value).toBe(""));
    expect(clear).toHaveBeenCalled();
    expect(resetHeight).toHaveBeenCalled();
  });
});
