import { act, cleanup, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { ToastProvider } from "@/components/toast-provider";
import { useChatInputState } from "./use-chat-input-state";
import { MAX_FILES, MAX_FILE_SIZE, MAX_TOTAL_SIZE } from "./file-attachment";
import type { TipTapInputHandle } from "./tiptap-input";
import type { EntityReference } from "@/lib/types/entity-reference";

type SubmitHandler = Parameters<typeof useChatInputState>[0]["onSubmit"];

function renderInputState(onSubmit: SubmitHandler) {
  return renderHook(
    () =>
      useChatInputState({
        sessionId: "session-1",
        isSending: false,
        contextItems: [],
        showRequestChangesTooltip: false,
        onSubmit,
      }),
    {
      wrapper: ({ children }) => React.createElement(ToastProvider, null, children),
    },
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

function fileWithSize(name: string, size: number) {
  const file = new File(["attachment"], name, { type: "text/plain" });
  Object.defineProperty(file, "size", { value: size });
  return file;
}

const attachmentCountLimitMessage = `You can attach up to ${MAX_FILES} files.`;

function toastMessage() {
  return screen.getByTestId("toast-message").textContent;
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

afterEach(cleanup);

describe("useChatInputState", () => {
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

describe("useChatInputState attachment feedback", () => {
  it("warns when a batch exceeds the maximum number of files", async () => {
    const { result } = renderInputState(vi.fn());
    const files = Array.from({ length: MAX_FILES + 1 }, (_, index) =>
      fileWithSize(`attachment-${index}.txt`, 1),
    );

    await act(async () => {
      await result.current.addFiles(files);
    });

    expect(result.current.attachments).toHaveLength(MAX_FILES);
    expect(toastMessage()).toContain(attachmentCountLimitMessage);
  });

  it("warns when adding files after reaching the maximum number of files", async () => {
    const { result } = renderInputState(vi.fn());
    const files = Array.from({ length: MAX_FILES }, (_, index) =>
      fileWithSize(`attachment-${index}.txt`, 1),
    );

    await act(async () => {
      await result.current.addFiles(files);
    });
    await waitFor(() => expect(result.current.attachments).toHaveLength(MAX_FILES));

    await act(async () => {
      await result.current.addFiles([fileWithSize("one-too-many.txt", 1)]);
    });

    expect(result.current.attachments).toHaveLength(MAX_FILES);
    expect(toastMessage()).toContain(attachmentCountLimitMessage);
  });

  it("accepts only files that fit within the total size limit and warns once", async () => {
    const { result } = renderInputState(vi.fn());
    const fileSize = MAX_TOTAL_SIZE / 3 + 1;
    const files = [
      fileWithSize("first.txt", fileSize),
      fileWithSize("second.txt", fileSize),
      fileWithSize("third.txt", fileSize),
    ];

    await act(async () => {
      await result.current.addFiles(files);
    });

    expect(result.current.attachments).toHaveLength(2);
    expect(screen.getAllByTestId("toast-message")).toHaveLength(1);
    expect(toastMessage()).toContain("Attachment limit reached");
    expect(toastMessage()).toContain(
      `Attachments can total up to ${MAX_TOTAL_SIZE / 1024 / 1024} MB.`,
    );
  });

  it("warns when a pasted attachment exceeds the file size limit", async () => {
    const { result } = renderInputState(vi.fn());
    const oversizedFile = new File(["video"], "recording.mov", { type: "video/quicktime" });
    Object.defineProperty(oversizedFile, "size", { value: 11 * 1024 * 1024 });

    await act(async () => {
      await result.current.addFiles([oversizedFile]);
    });

    expect(toastMessage()).toContain("Attachment is too large");
    expect(toastMessage()).toContain(
      `recording.mov is 11 MB. The maximum file size is ${MAX_FILE_SIZE / 1024 / 1024} MB.`,
    );
    expect(result.current.attachments).toEqual([]);
  });

  it("warns when a pasted image has no readable file data", async () => {
    const { result } = renderInputState(vi.fn());

    await act(async () => {
      await result.current.addFiles([], "unreadable-image");
    });

    expect(toastMessage()).toContain("Pasted image couldn’t be attached");
    expect(toastMessage()).toContain(
      "The browser didn’t provide image data. Save the image, then attach the file instead.",
    );
    expect(result.current.attachments).toEqual([]);
  });
});
