import { act, cleanup, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { ToastProvider } from "@/components/toast-provider";
import { useChatInputState } from "./use-chat-input-state";
import { MAX_FILES, MAX_FILE_SIZE, MAX_TOTAL_SIZE } from "./file-attachment";
import { formatBytes } from "@/lib/utils/format-bytes";
import type { TipTapInputHandle } from "./tiptap-input";
import type { EntityReference } from "@/lib/types/entity-reference";
import type { FileAttachment } from "./file-attachment";
import * as attachmentFiles from "./file-attachment";
import { setChatDraftAttachments } from "@/lib/local-storage";

const uploadAttachmentMock = vi.hoisted(() => vi.fn());
const deleteAttachmentMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/attachment-api", () => ({
  uploadAttachment: uploadAttachmentMock,
  deleteAttachment: deleteAttachmentMock,
}));

type SubmitHandler = Parameters<typeof useChatInputState>[0]["onSubmit"];
const ATTACHMENT_CONTENT = "attachment";
const TEXT_MIME_TYPE = "text/plain";
const UPLOADED_ATTACHMENT_ID = "uploaded-attachment";
const WORKSPACE_ONE = "workspace-1";
const KEEP_DRAFT_TEXT = "keep this draft";

function renderInputState(
  onSubmit: SubmitHandler,
  options: { workspaceId?: string | null; taskId?: string | null; sessionId?: string | null } = {},
) {
  return renderHook(
    () =>
      useChatInputState({
        sessionId: options.sessionId ?? "session-1",
        workspaceId: options.workspaceId,
        taskId: options.taskId,
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
const REFERENCE_MARKDOWN = "[#acme/repo#42](https://github.com/acme/repo/issues/42)";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function processedFile(file: File, id: string): FileAttachment {
  return {
    id,
    file,
    mimeType: file.type || "application/octet-stream",
    fileName: file.name,
    size: file.size,
    isImage: false,
    deliveryMode: "path",
    uploadStatus: "pending",
  };
}

function fileWithSize(name: string, size: number) {
  const file = new File([ATTACHMENT_CONTENT], name, { type: TEXT_MIME_TYPE });
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
  uploadAttachmentMock.mockReset().mockResolvedValue({
    attachment_id: UPLOADED_ATTACHMENT_ID,
    name: "notes.txt",
    mime_type: TEXT_MIME_TYPE,
    kind: "resource",
    delivery_mode: "path",
    size_bytes: 10,
  });
  deleteAttachmentMock.mockReset().mockResolvedValue(undefined);
});

afterEach(cleanup);

// eslint-disable-next-line max-lines-per-function -- input draft and attachment cases share one state contract.
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

  it("keeps staged attachments when async submit reports failure", async () => {
    const onSubmit = vi
      .fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>()
      .mockResolvedValue(false);
    const clear = vi.fn();
    const { result } = renderInputState(onSubmit, { workspaceId: WORKSPACE_ONE });

    await act(async () => {
      await result.current.addFiles([
        new File([ATTACHMENT_CONTENT], "notes.txt", { type: TEXT_MIME_TYPE }),
      ]);
    });
    await waitFor(() => expect(result.current.attachments[0]?.attachmentId).toBeTruthy());

    act(() => {
      result.current.handleChange("keep this with the file");
      attachInputHandle(result.current.inputRef, clear);
      result.current.handleSubmit(vi.fn());
    });

    await waitFor(() => expect(onSubmit).toHaveBeenCalled());
    expect(result.current.value).toBe("keep this with the file");
    expect(result.current.attachments).toHaveLength(1);
    expect(clear).not.toHaveBeenCalled();
  });

  it("captures structured entity references in the named submit payload", async () => {
    const onSubmit = vi.fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>();
    const { result } = renderInputState(onSubmit);

    act(() => {
      result.current.handleChange(REFERENCE_MARKDOWN);
      attachInputHandle(result.current.inputRef, vi.fn(), [reference]);
    });
    await waitFor(() => expect(result.current.value).toContain("acme/repo#42"));

    act(() => {
      result.current.handleSubmit(vi.fn());
    });

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith({
        message: REFERENCE_MARKDOWN,
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
        new File(["new attachment"], "later.txt", { type: TEXT_MIME_TYPE }),
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

describe("useChatInputState immediate submission", () => {
  it("submits a structured reference immediately after the editor change", () => {
    const onSubmit = vi.fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>();
    const { result } = renderInputState(onSubmit);

    act(() => {
      result.current.handleChange(REFERENCE_MARKDOWN);
      attachInputHandle(result.current.inputRef, vi.fn(), [reference]);
      result.current.handleSubmit(vi.fn());
    });

    expect(onSubmit).toHaveBeenCalledWith({
      message: REFERENCE_MARKDOWN,
      entityReferences: [reference],
    });
  });
});

// eslint-disable-next-line max-lines-per-function -- these cases cover the attachment upload lifecycle.
describe("useChatInputState attachment feedback", () => {
  it("allows a plain-text message while task workspace scope is unresolved", () => {
    const onSubmit = vi.fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>();
    const { result } = renderInputState(onSubmit);

    act(() => {
      result.current.handleChange("send text without a file");
      attachInputHandle(result.current.inputRef, vi.fn());
      result.current.handleSubmit(vi.fn());
    });

    expect(onSubmit).toHaveBeenCalledWith({ message: "send text without a file" });
  });

  it("blocks file submission until an attachment descriptor is ready, even before scope resolves", async () => {
    const onSubmit = vi.fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>();
    const { result } = renderInputState(onSubmit);

    await act(async () => {
      await result.current.addFiles([
        new File([ATTACHMENT_CONTENT], "notes.txt", { type: TEXT_MIME_TYPE }),
      ]);
    });
    act(() => {
      result.current.handleChange(KEEP_DRAFT_TEXT);
      attachInputHandle(result.current.inputRef, vi.fn());
      result.current.handleSubmit(vi.fn());
    });

    expect(onSubmit).not.toHaveBeenCalled();
    expect(result.current.hasPendingAttachmentUploads).toBe(true);
    expect(result.current.value).toBe(KEEP_DRAFT_TEXT);
  });

  // @covers AC-TASKS-PROMPT-ATTACHMENTS-001.16
  it("uploads a restored file draft before allowing submission", async () => {
    setChatDraftAttachments("session-1", [
      {
        id: "restored-file",
        data: btoa(ATTACHMENT_CONTENT),
        mimeType: TEXT_MIME_TYPE,
        fileName: "notes.txt",
        size: ATTACHMENT_CONTENT.length,
        isImage: false,
        deliveryMode: "path",
      },
    ]);
    const onSubmit = vi.fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>();
    const scope = { workspaceId: null as string | null };
    const { result, rerender } = renderInputState(onSubmit, scope);

    expect(result.current.hasPendingAttachmentUploads).toBe(true);
    expect(result.current.attachments[0]?.file).toBeInstanceOf(File);
    attachInputHandle(result.current.inputRef, vi.fn());
    act(() => result.current.handleSubmit(vi.fn()));
    expect(onSubmit).not.toHaveBeenCalled();

    act(() => {
      scope.workspaceId = WORKSPACE_ONE;
      rerender();
    });
    await waitFor(() => expect(result.current.attachments[0]?.attachmentId).toBeTruthy());
    expect(uploadAttachmentMock).toHaveBeenCalledOnce();

    act(() => result.current.handleSubmit(vi.fn()));
    expect(onSubmit).toHaveBeenCalledWith({
      message: "",
      attachments: [
        expect.objectContaining({ attachment_id: UPLOADED_ATTACHMENT_ID, name: "notes.txt" }),
      ],
    });
  });

  it("starts the upload and allows file submission when scope resolves", async () => {
    const onSubmit = vi.fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>();
    const scope = { workspaceId: null as string | null };
    const { result, rerender } = renderInputState(onSubmit, scope);

    await act(async () => {
      await result.current.addFiles([
        new File([ATTACHMENT_CONTENT], "notes.txt", { type: TEXT_MIME_TYPE }),
      ]);
    });
    expect(uploadAttachmentMock).not.toHaveBeenCalled();
    expect(result.current.hasPendingAttachmentUploads).toBe(true);

    act(() => {
      result.current.handleChange("keep this draft");
      attachInputHandle(result.current.inputRef, vi.fn());
      scope.workspaceId = WORKSPACE_ONE;
      rerender();
    });
    await waitFor(() => expect(result.current.attachments[0]?.attachmentId).toBeTruthy());
    expect(uploadAttachmentMock).toHaveBeenCalledWith(expect.any(File), {
      workspaceId: WORKSPACE_ONE,
      kind: "resource",
      deliveryMode: "path",
    });

    act(() => result.current.handleSubmit(vi.fn()));
    expect(onSubmit).toHaveBeenCalledWith({
      message: KEEP_DRAFT_TEXT,
      attachments: [
        expect.objectContaining({ attachment_id: UPLOADED_ATTACHMENT_ID, name: "notes.txt" }),
      ],
    });
  });

  it("uploads a file decoded after workspace scope resolves", async () => {
    const processing = deferred<FileAttachment | null>();
    const process = vi
      .spyOn(attachmentFiles, "processFile")
      .mockReturnValueOnce(processing.promise);
    const scope = { workspaceId: null as string | null };
    const { result, rerender } = renderInputState(vi.fn(), scope);
    const file = new File([ATTACHMENT_CONTENT], "delayed.txt", { type: TEXT_MIME_TYPE });
    let adding!: Promise<void>;

    try {
      act(() => {
        adding = result.current.addFiles([file]);
      });
      expect(uploadAttachmentMock).not.toHaveBeenCalled();

      act(() => {
        scope.workspaceId = WORKSPACE_ONE;
        rerender();
      });
      expect(uploadAttachmentMock).not.toHaveBeenCalled();

      await act(async () => {
        processing.resolve(processedFile(file, "delayed-file"));
        await adding;
      });

      await waitFor(() => expect(result.current.attachments[0]?.attachmentId).toBeTruthy());
      expect(uploadAttachmentMock).toHaveBeenCalledOnce();
      expect(uploadAttachmentMock).toHaveBeenCalledWith(file, {
        workspaceId: WORKSPACE_ONE,
        kind: "resource",
        deliveryMode: "path",
      });
    } finally {
      process.mockRestore();
    }
  });

  it("uploads a file decoded before workspace scope resolves", async () => {
    const processing = deferred<FileAttachment | null>();
    const process = vi
      .spyOn(attachmentFiles, "processFile")
      .mockReturnValueOnce(processing.promise);
    const scope = { workspaceId: null as string | null };
    const { result, rerender } = renderInputState(vi.fn(), scope);
    const file = new File([ATTACHMENT_CONTENT], "scope-later.txt", { type: TEXT_MIME_TYPE });
    let adding!: Promise<void>;

    try {
      act(() => {
        adding = result.current.addFiles([file]);
      });
      await act(async () => {
        processing.resolve(processedFile(file, "scope-later-file"));
        await adding;
      });
      expect(uploadAttachmentMock).not.toHaveBeenCalled();

      act(() => {
        scope.workspaceId = WORKSPACE_ONE;
        rerender();
      });

      await waitFor(() => expect(result.current.attachments[0]?.attachmentId).toBeTruthy());
      expect(uploadAttachmentMock).toHaveBeenCalledOnce();
    } finally {
      process.mockRestore();
    }
  });

  it("discards a file decoder result after its draft identity changes", async () => {
    const processing = deferred<FileAttachment | null>();
    const process = vi
      .spyOn(attachmentFiles, "processFile")
      .mockReturnValueOnce(processing.promise);
    const scope = { workspaceId: WORKSPACE_ONE, taskId: "task-1", sessionId: "session-1" };
    const { result, rerender } = renderInputState(vi.fn(), scope);
    const file = new File([ATTACHMENT_CONTENT], "old-draft.txt", { type: TEXT_MIME_TYPE });
    let adding!: Promise<void>;

    try {
      act(() => {
        adding = result.current.addFiles([file]);
      });
      scope.taskId = "task-2";
      scope.sessionId = "session-2";
      rerender();

      await act(async () => {
        processing.resolve(processedFile(file, "old-draft-file"));
        await adding;
      });

      expect(result.current.attachments).toEqual([]);
      expect(uploadAttachmentMock).not.toHaveBeenCalled();
    } finally {
      process.mockRestore();
    }
  });

  it("allows an uploaded attachment to submit without text", async () => {
    const onSubmit = vi.fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>();
    const { result } = renderInputState(onSubmit, { workspaceId: WORKSPACE_ONE });

    await act(async () => {
      await result.current.addFiles([
        new File([ATTACHMENT_CONTENT], "notes.txt", { type: TEXT_MIME_TYPE }),
      ]);
    });
    await waitFor(() => expect(result.current.attachments[0]?.attachmentId).toBeTruthy());
    attachInputHandle(result.current.inputRef, vi.fn());

    act(() => result.current.handleSubmit(vi.fn()));

    expect(onSubmit).toHaveBeenCalledWith({
      message: "",
      attachments: [
        expect.objectContaining({ attachment_id: UPLOADED_ATTACHMENT_ID, name: "notes.txt" }),
      ],
    });
  });

  it("keeps a failed upload without automatic retries and supports explicit retry", async () => {
    uploadAttachmentMock
      .mockRejectedValueOnce(new Error("temporary upload error"))
      .mockResolvedValueOnce({
        attachment_id: "retried-attachment",
        name: "notes.txt",
        mime_type: TEXT_MIME_TYPE,
        kind: "resource",
        delivery_mode: "path",
        size_bytes: 10,
      });
    const onSubmit = vi.fn<(...args: Parameters<SubmitHandler>) => ReturnType<SubmitHandler>>();
    const { result } = renderInputState(onSubmit, { workspaceId: WORKSPACE_ONE });

    await act(async () => {
      await result.current.addFiles([
        new File([ATTACHMENT_CONTENT], "notes.txt", { type: TEXT_MIME_TYPE }),
      ]);
    });
    await waitFor(() => expect(result.current.attachments[0]?.uploadStatus).toBe("failed"));
    act(() => {
      result.current.handleChange(KEEP_DRAFT_TEXT);
      attachInputHandle(result.current.inputRef, vi.fn());
      result.current.handleSubmit(vi.fn());
    });

    expect(onSubmit).not.toHaveBeenCalled();
    expect(result.current.value).toBe(KEEP_DRAFT_TEXT);
    expect(result.current.hasPendingAttachmentUploads).toBe(true);
    expect(uploadAttachmentMock).toHaveBeenCalledOnce();

    const attachmentItem = result.current.allItems[0];
    expect(attachmentItem?.kind).toBe("file-attachment");
    const retry = attachmentItem?.kind === "file-attachment" ? attachmentItem.onRetry : undefined;
    expect(retry).toBeTypeOf("function");
    if (retry) act(retry);
    await waitFor(() =>
      expect(result.current.attachments[0]?.attachmentId).toBe("retried-attachment"),
    );
    expect(uploadAttachmentMock).toHaveBeenCalledTimes(2);
  });

  it("discards and deletes an upload that finishes after the session and task change", async () => {
    const upload = deferred<{
      attachment_id: string;
      name: string;
      mime_type: string;
      kind: "resource";
      delivery_mode: "path";
      size_bytes: number;
    }>();
    uploadAttachmentMock.mockReturnValue(upload.promise);
    const scope = {
      workspaceId: WORKSPACE_ONE,
      taskId: "task-1",
      sessionId: "session-1",
    };
    const { result, rerender } = renderInputState(vi.fn(), scope);

    await act(async () => {
      await result.current.addFiles([
        new File([ATTACHMENT_CONTENT], "notes.txt", { type: TEXT_MIME_TYPE }),
      ]);
    });
    await waitFor(() => expect(uploadAttachmentMock).toHaveBeenCalledOnce());

    scope.workspaceId = "workspace-2";
    scope.taskId = "task-2";
    scope.sessionId = "session-2";
    rerender();
    await waitFor(() => expect(result.current.attachments).toEqual([]));

    await act(async () => {
      upload.resolve({
        attachment_id: "late-attachment",
        name: "notes.txt",
        mime_type: TEXT_MIME_TYPE,
        kind: "resource",
        delivery_mode: "path",
        size_bytes: 10,
      });
      await upload.promise;
    });

    await waitFor(() => expect(deleteAttachmentMock).toHaveBeenCalledWith("late-attachment"));
  });

  it("clears ready attachments when the task changes but the session ID stays the same", async () => {
    const scope = {
      workspaceId: WORKSPACE_ONE,
      taskId: "task-1",
      sessionId: "session-1",
    };
    const { result, rerender } = renderInputState(vi.fn(), scope);

    await act(async () => {
      await result.current.addFiles([
        new File([ATTACHMENT_CONTENT], "notes.txt", { type: TEXT_MIME_TYPE }),
      ]);
    });
    await waitFor(() => expect(result.current.attachments[0]?.attachmentId).toBeTruthy());

    scope.taskId = "task-2";
    rerender();

    await waitFor(() => expect(result.current.attachments).toEqual([]));
    await waitFor(() => expect(deleteAttachmentMock).toHaveBeenCalledWith(UPLOADED_ATTACHMENT_ID));
  });

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
    expect(toastMessage()).toContain(`Attachments can total up to ${formatBytes(MAX_TOTAL_SIZE)}.`);
  });

  it("warns when a pasted attachment exceeds the file size limit", async () => {
    const { result } = renderInputState(vi.fn());
    const oversizedFile = new File(["video"], "recording.mov", { type: "video/quicktime" });
    Object.defineProperty(oversizedFile, "size", { value: MAX_FILE_SIZE + 1 });

    await act(async () => {
      await result.current.addFiles([oversizedFile]);
    });

    expect(toastMessage()).toContain("Attachment is too large");
    expect(toastMessage()).toContain(
      `recording.mov is ${formatBytes(MAX_FILE_SIZE + 1)}. The maximum file size is ${formatBytes(MAX_FILE_SIZE)}.`,
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
