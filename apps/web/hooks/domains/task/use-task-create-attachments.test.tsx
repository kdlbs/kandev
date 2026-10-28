import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import {
  attachmentSubmissionBlocker,
  restoreTaskCreateAttachments,
  toStoredCreationAttachments,
  useTaskCreateAttachments,
} from "./use-task-create-attachments";

const { uploadAttachment, deleteAttachment, processFile } = vi.hoisted(() => ({
  uploadAttachment: vi.fn(),
  deleteAttachment: vi.fn().mockResolvedValue(undefined),
  processFile: vi.fn(),
}));

vi.mock("@/lib/api/domains/attachment-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/attachment-api")>();
  return { ...actual, uploadAttachment, deleteAttachment };
});

vi.mock("@/components/task/chat/file-attachment", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/task/chat/file-attachment")>();
  processFile.mockImplementation(actual.processFile);
  return { ...actual, processFile };
});

function wrapper({ children }: { children: ReactNode }) {
  return <ToastProvider>{children}</ToastProvider>;
}

const WORKSPACE_ID = "ws-1";
const FILE_NAME = "notes.txt";
const TEXT_MIME = "text/plain";
const READY_ID = "attachment-ready";

function textFile(name = FILE_NAME, contents = "hello") {
  return new File([contents], name, { type: TEXT_MIME });
}

function uploadedDescriptor(id = "attachment-1", size = 5) {
  return {
    attachment_id: id,
    name: FILE_NAME,
    mime_type: TEXT_MIME,
    kind: "resource" as const,
    delivery_mode: "path" as const,
    size_bytes: size,
    expires_at: new Date(Date.now() + 60_000).toISOString(),
  };
}

afterEach(() => {
  uploadAttachment.mockReset();
  deleteAttachment.mockClear();
});

// eslint-disable-next-line max-lines-per-function -- lifecycle cases share one upload mock and provider.
describe("useTaskCreateAttachments", () => {
  it("uploads a selected file and exposes a ready descriptor", async () => {
    uploadAttachment.mockResolvedValue(uploadedDescriptor());
    const { result } = renderHook(() => useTaskCreateAttachments({ workspaceId: WORKSPACE_ID }), {
      wrapper,
    });

    await act(async () => {
      await result.current.addFiles([textFile()]);
    });

    await waitFor(() => expect(result.current.attachments[0]?.uploadStatus).toBe("ready"));
    expect(uploadAttachment).toHaveBeenCalledWith(
      expect.any(File),
      expect.objectContaining({
        workspaceId: WORKSPACE_ID,
        kind: "resource",
        deliveryMode: "path",
      }),
    );
    expect(attachmentSubmissionBlocker(result.current.attachments)).toBeNull();
  });

  it("blocks submission while an upload is in flight", async () => {
    let resolveUpload: (value: ReturnType<typeof uploadedDescriptor>) => void = () => undefined;
    uploadAttachment.mockReturnValue(
      new Promise((resolve) => {
        resolveUpload = resolve;
      }),
    );
    const { result } = renderHook(() => useTaskCreateAttachments({ workspaceId: WORKSPACE_ID }), {
      wrapper,
    });

    await act(async () => {
      await result.current.addFiles([textFile()]);
    });
    await waitFor(() => expect(result.current.attachments[0]?.uploadStatus).toBe("uploading"));
    expect(attachmentSubmissionBlocker(result.current.attachments)).toBe("uploading");

    await act(async () => {
      resolveUpload(uploadedDescriptor());
    });
    await waitFor(() => expect(result.current.attachments[0]?.uploadStatus).toBe("ready"));
  });

  it("shows a failed upload, retries it, and preserves ready siblings", async () => {
    uploadAttachment
      .mockResolvedValueOnce(uploadedDescriptor(READY_ID, 4))
      .mockRejectedValueOnce(new Error("network down"))
      .mockResolvedValueOnce(uploadedDescriptor("attachment-retry", 7));
    const { result } = renderHook(() => useTaskCreateAttachments({ workspaceId: WORKSPACE_ID }), {
      wrapper,
    });

    await act(async () => {
      await result.current.addFiles([
        new File(["good"], "good.txt", { type: TEXT_MIME }),
        new File(["retry"], "retry.txt", { type: TEXT_MIME }),
      ]);
    });
    await waitFor(() =>
      expect(result.current.attachments.map((att) => att.uploadStatus)).toEqual([
        "ready",
        "failed",
      ]),
    );
    expect(attachmentSubmissionBlocker(result.current.attachments)).toBe("failed");

    await act(async () => {
      result.current.handleRetryAttachment(result.current.attachments[1].id);
    });
    await waitFor(() =>
      expect(result.current.attachments.map((att) => att.uploadStatus)).toEqual(["ready", "ready"]),
    );
    expect(result.current.attachments[0].attachmentId).toBe(READY_ID);
  });

  it("deletes a late upload result after its chip was removed", async () => {
    let resolveUpload: (value: ReturnType<typeof uploadedDescriptor>) => void = () => undefined;
    uploadAttachment.mockReturnValue(
      new Promise((resolve) => {
        resolveUpload = resolve;
      }),
    );
    const { result } = renderHook(() => useTaskCreateAttachments({ workspaceId: WORKSPACE_ID }), {
      wrapper,
    });
    await act(async () => {
      await result.current.addFiles([textFile()]);
    });
    await waitFor(() => expect(result.current.attachments).toHaveLength(1));
    const id = result.current.attachments[0].id;

    act(() => result.current.handleRemoveAttachment(id));
    await act(async () => {
      resolveUpload(uploadedDescriptor("late-attachment"));
    });

    await waitFor(() => expect(deleteAttachment).toHaveBeenCalledWith("late-attachment"));
    expect(result.current.attachments).toHaveLength(0);
  });

  it("blocks submission while file preprocessing is pending and discards stale results", async () => {
    let resolveProcessing: (value: unknown) => void = () => undefined;
    processFile.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveProcessing = resolve;
        }),
    );
    uploadAttachment.mockResolvedValue(uploadedDescriptor());
    const { result } = renderHook(() => useTaskCreateAttachments({ workspaceId: WORKSPACE_ID }), {
      wrapper,
    });

    let pendingAdd: Promise<void> = Promise.resolve();
    act(() => {
      pendingAdd = result.current.addFiles([textFile("stale.txt")]);
    });
    await waitFor(() => expect(result.current.isProcessing).toBe(true));
    expect(
      attachmentSubmissionBlocker(result.current.attachments, result.current.isProcessing),
    ).toBe("uploading");

    act(() => result.current.clearAttachments());
    await act(async () => {
      resolveProcessing({
        id: "stale-attachment",
        file: textFile("stale.txt"),
        mimeType: TEXT_MIME,
        fileName: "stale.txt",
        size: 5,
        isImage: false,
        deliveryMode: "path",
        uploadStatus: "pending",
      });
      await pendingAdd;
    });
    expect(result.current.attachments).toHaveLength(0);
    expect(uploadAttachment).not.toHaveBeenCalled();

    await act(async () => {
      await result.current.addFiles([textFile("successor.txt")]);
    });
    await waitFor(() => expect(result.current.attachments).toHaveLength(1));
    expect(result.current.attachments[0].fileName).toBe("successor.txt");
    expect(uploadAttachment).toHaveBeenCalledTimes(1);
  });

  it("notifies the owner when a ready attachment is removed or cleared", async () => {
    uploadAttachment
      .mockResolvedValueOnce(uploadedDescriptor("attachment-a", 4))
      .mockResolvedValueOnce(uploadedDescriptor("attachment-b", 5));
    const changes: string[][] = [];
    const { result } = renderHook(
      () =>
        useTaskCreateAttachments({
          workspaceId: WORKSPACE_ID,
          onAttachmentsChange: (attachments) => {
            changes.push(attachments.map((attachment) => attachment.fileName));
          },
        }),
      { wrapper },
    );

    await act(async () => {
      await result.current.addFiles([
        new File(["aaaa"], "a.txt", { type: TEXT_MIME }),
        new File(["bbbbb"], "b.txt", { type: TEXT_MIME }),
      ]);
    });
    await waitFor(() =>
      expect(result.current.attachments.map((attachment) => attachment.uploadStatus)).toEqual([
        "ready",
        "ready",
      ]),
    );

    act(() => result.current.handleRemoveAttachment(result.current.attachments[0].id));
    expect(changes.at(-1)).toEqual(["b.txt"]);

    act(() => result.current.clearAttachments({ deleteStaged: false }));
    expect(changes.at(-1)).toEqual([]);
    expect(result.current.attachments).toHaveLength(0);
  });

  it("blocks removal and retry while submission is frozen", async () => {
    uploadAttachment.mockResolvedValue(uploadedDescriptor());
    const { result } = renderHook(() => useTaskCreateAttachments({ workspaceId: WORKSPACE_ID }), {
      wrapper,
    });
    await act(async () => {
      await result.current.addFiles([textFile()]);
    });
    await waitFor(() => expect(result.current.attachments[0]?.uploadStatus).toBe("ready"));
    const id = result.current.attachments[0].id;

    act(() => result.current.setAccepting(false));
    act(() => result.current.handleRemoveAttachment(id));
    act(() => result.current.handleRetryAttachment(id));
    expect(result.current.attachments).toHaveLength(1);
    expect(deleteAttachment).not.toHaveBeenCalled();
  });

  it("rejects late upload results after the owner unmounts", async () => {
    let resolveUpload: (value: ReturnType<typeof uploadedDescriptor>) => void = () => undefined;
    uploadAttachment.mockReturnValue(
      new Promise((resolve) => {
        resolveUpload = resolve;
      }),
    );
    const { result, unmount } = renderHook(
      () => useTaskCreateAttachments({ workspaceId: WORKSPACE_ID, invalidateOnUnmount: true }),
      { wrapper },
    );
    await act(async () => {
      await result.current.addFiles([textFile()]);
    });
    await waitFor(() => expect(result.current.attachments[0]?.uploadStatus).toBe("uploading"));
    unmount();
    await act(async () => {
      resolveUpload(uploadedDescriptor("late-unmounted"));
    });
    await waitFor(() => expect(deleteAttachment).toHaveBeenCalledWith("late-unmounted"));
  });

  it("stops accepting new files when submission freezes the composer", async () => {
    uploadAttachment.mockResolvedValue(uploadedDescriptor());
    const { result } = renderHook(() => useTaskCreateAttachments({ workspaceId: WORKSPACE_ID }), {
      wrapper,
    });
    act(() => result.current.setAccepting(false));
    await act(async () => {
      await result.current.addFiles([textFile("blocked.txt")]);
    });
    expect(result.current.attachments).toHaveLength(0);
    expect(uploadAttachment).not.toHaveBeenCalled();

    act(() => result.current.setAccepting(true));
    await act(async () => {
      await result.current.addFiles([textFile("allowed.txt")]);
    });
    await waitFor(() => expect(result.current.attachments).toHaveLength(1));
    expect(uploadAttachment).toHaveBeenCalledTimes(1);
  });

  it("clears staged files on discard but not after a successful claim", async () => {
    uploadAttachment.mockResolvedValue(uploadedDescriptor());
    const { result } = renderHook(() => useTaskCreateAttachments({ workspaceId: WORKSPACE_ID }), {
      wrapper,
    });
    await act(async () => {
      await result.current.addFiles([textFile()]);
    });
    await waitFor(() => expect(result.current.attachments[0]?.uploadStatus).toBe("ready"));

    act(() => result.current.clearAttachments({ deleteStaged: false }));
    expect(deleteAttachment).not.toHaveBeenCalled();
    expect(result.current.attachments).toHaveLength(0);

    await act(async () => {
      await result.current.addFiles([textFile("second.txt")]);
    });
    await waitFor(() => expect(result.current.attachments[0]?.uploadStatus).toBe("ready"));
    act(() => result.current.clearAttachments());
    await waitFor(() => expect(deleteAttachment).toHaveBeenCalledWith("attachment-1"));
  });
});

describe("toStoredCreationAttachments", () => {
  it("drops inline bytes and in-flight files", () => {
    const stored = toStoredCreationAttachments([
      {
        id: "ready",
        attachmentId: READY_ID,
        data: "aGVsbG8=",
        file: new File(["hello"], FILE_NAME, { type: TEXT_MIME }),
        mimeType: TEXT_MIME,
        fileName: FILE_NAME,
        size: 5,
        isImage: false,
        deliveryMode: "path",
        uploadStatus: "ready",
      },
      {
        id: "in-flight",
        file: new File(["pending"], "pending.txt", { type: TEXT_MIME }),
        mimeType: TEXT_MIME,
        fileName: "pending.txt",
        size: 7,
        isImage: false,
        deliveryMode: "path",
        uploadStatus: "uploading",
      },
    ]);
    expect(stored).toHaveLength(1);
    expect(stored[0]).toMatchObject({ id: "ready", attachmentId: "attachment-ready" });
    expect(stored[0]).not.toHaveProperty("data");
    expect(stored[0]).not.toHaveProperty("file");
  });
});

describe("restoreTaskCreateAttachments", () => {
  it("restores ready descriptors with an authorized preview", () => {
    const restored = restoreTaskCreateAttachments([
      {
        id: "stored-1",
        attachmentId: "attachment-1",
        expiresAt: new Date(Date.now() + 60_000).toISOString(),
        mimeType: "image/png",
        fileName: "screenshot.png",
        size: 10,
        isImage: true,
        deliveryMode: "prompt",
      },
    ]);
    expect(restored[0].uploadStatus).toBe("ready");
    expect(restored[0].preview).toContain("/api/v1/attachments/attachment-1/content");
  });

  it("marks expired restored descriptors as failed", () => {
    const restored = restoreTaskCreateAttachments([
      {
        id: "stored-2",
        attachmentId: "attachment-2",
        expiresAt: new Date(Date.now() - 60_000).toISOString(),
        mimeType: "image/png",
        fileName: "old.png",
        size: 10,
        isImage: true,
        deliveryMode: "prompt",
      },
    ]);
    expect(restored[0].uploadStatus).toBe("failed");
    expect(attachmentSubmissionBlocker(restored)).toBe("failed");
  });

  it("rejects malformed and normalized-invalid expiry values", () => {
    const restored = restoreTaskCreateAttachments([
      {
        id: "stored-bad",
        attachmentId: "attachment-bad",
        expiresAt: "not-a-date",
        mimeType: TEXT_MIME,
        fileName: "bad.txt",
        size: 1,
        isImage: false,
        deliveryMode: "path",
      },
      {
        id: "stored-normalized",
        attachmentId: "attachment-normalized",
        expiresAt: "2026-02-30T10:00:00Z",
        mimeType: TEXT_MIME,
        fileName: "normalized.txt",
        size: 1,
        isImage: false,
        deliveryMode: "path",
      },
    ]);
    expect(restored.map((attachment) => attachment.uploadStatus)).toEqual(["failed", "failed"]);
    expect(attachmentSubmissionBlocker(restored)).toBe("failed");
  });

  it("restores legacy inline bytes without persisting them", () => {
    const data = btoa("hello");
    const restored = restoreTaskCreateAttachments([
      {
        id: "stored-3",
        data,
        mimeType: TEXT_MIME,
        fileName: "legacy.txt",
        size: 5,
        isImage: false,
        deliveryMode: "path",
      },
    ]);
    expect(restored[0].uploadStatus).toBe("pending");
    expect(restored[0].file).toBeInstanceOf(File);
  });
});
