import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type React from "react";
import { useChatInputState } from "./use-chat-input-state";
import type { TipTapInputHandle } from "./tiptap-input";
import type { EntityReference } from "@/lib/types/entity-reference";

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
  clear: () => void,
  entityReferences: EntityReference[] = [],
) {
  (inputRef as React.MutableRefObject<Partial<TipTapInputHandle> | null>).current = {
    clear,
    getMentions: () => [],
    getTaskMentions: () => [],
    getEntityReferences: () => entityReferences,
  };
}

const reference: EntityReference = {
  version: 1,
  ref: "mention:v1:github:issue:acme%2Frepo:42",
  provider: "github",
  kind: "issue",
  id: "42",
  key: "acme/repo#42",
  title: "Fix composer references",
  url: "https://github.com/acme/repo/issues/42",
  scope: "acme/repo",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe("useChatInputState", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("keeps the draft when async submit reports failure", async () => {
    const onSubmit = vi
      .fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>()
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

    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith({ message: "hello" }));
    expect(result.current.value).toBe("hello");
    expect(clear).not.toHaveBeenCalled();
  });

  it("captures structured entity references in the named submit payload", async () => {
    const onSubmit = vi.fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>();
    const { result } = renderInputState(onSubmit);

    act(() => {
      result.current.handleChange("[#acme/repo#42](https://github.com/acme/repo/issues/42)");
      attachInputHandle(result.current.inputRef, vi.fn(), [reference]);
    });
    await waitFor(() => expect(result.current.value).toContain("acme/repo#42"));

    act(() => {
      result.current.handleSubmit(vi.fn());
    });

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith({
        message: "[#acme/repo#42](https://github.com/acme/repo/issues/42)",
        entityReferences: [reference],
      }),
    );
  });

  it("clears the draft when async submit succeeds", async () => {
    const onSubmit = vi
      .fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>()
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

  it("keeps newer attachments when async submit succeeds after attachments change", async () => {
    const submit = deferred<boolean>();
    const onSubmit = vi
      .fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>()
      .mockReturnValue(submit.promise);
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
    await waitFor(() => expect(onSubmit).toHaveBeenCalled());

    await act(async () => {
      await result.current.addFiles([
        new File(["new attachment"], "later.txt", { type: "text/plain" }),
      ]);
    });
    await waitFor(() => expect(result.current.allItems).toHaveLength(1));

    await act(async () => {
      submit.resolve(true);
      await submit.promise;
    });

    await waitFor(() => expect(result.current.value).toBe(""));
    expect(result.current.allItems).toHaveLength(1);
    expect(clear).toHaveBeenCalled();
    expect(resetHeight).toHaveBeenCalled();
  });
});
