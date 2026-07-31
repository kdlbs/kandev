import { act, cleanup, renderHook, screen, waitFor } from "@testing-library/react";
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import { MAX_FILES, MAX_FILE_SIZE, MAX_TOTAL_SIZE } from "./chat/file-attachment";
import { formatBytes } from "@/lib/utils/format-bytes";
import { useDialogAttachments } from "./session-dialog-shared";

afterEach(cleanup);

const toastMessageTestId = "toast-message";

function fileWithSize(name: string, size: number) {
  const file = new File(["attachment"], name, { type: "text/plain" });
  Object.defineProperty(file, "size", { value: size });
  return file;
}

describe("useDialogAttachments", () => {
  it("warns when an image clipboard item has no readable file", async () => {
    const preventDefault = vi.fn();
    const clipboardData = {
      files: [],
      items: [{ kind: "file", type: "image/png", getAsFile: () => null }],
      getData: () => "",
    } as unknown as DataTransfer;
    const { result } = renderHook(() => useDialogAttachments(false), {
      wrapper: ({ children }) => React.createElement(ToastProvider, null, children),
    });

    await act(async () => {
      result.current.handlePaste({
        clipboardData,
        preventDefault,
      } as unknown as React.ClipboardEvent<HTMLTextAreaElement>);
    });

    expect(preventDefault).toHaveBeenCalledOnce();
    expect(screen.getByTestId(toastMessageTestId).textContent).toContain(
      "Pasted image couldn’t be attached",
    );
    expect(result.current.attachments).toEqual([]);
  });

  it("warns for an oversized clipboard file when item conversion provides no file", async () => {
    const oversizedImage = new File(["image"], "copied.png", { type: "image/png" });
    Object.defineProperty(oversizedImage, "size", { value: MAX_FILE_SIZE + 1 });
    const preventDefault = vi.fn();
    const clipboardData = {
      files: [oversizedImage],
      items: [{ kind: "file", type: "image/png", getAsFile: () => null }],
      getData: () => "",
    } as unknown as DataTransfer;
    const { result } = renderHook(() => useDialogAttachments(false), {
      wrapper: ({ children }) => React.createElement(ToastProvider, null, children),
    });

    await act(async () => {
      result.current.handlePaste({
        clipboardData,
        preventDefault,
      } as unknown as React.ClipboardEvent<HTMLTextAreaElement>);
    });

    expect(preventDefault).toHaveBeenCalledOnce();
    expect(screen.getByTestId(toastMessageTestId).textContent).toContain("Attachment is too large");
    expect(result.current.attachments).toEqual([]);
  });

  it("keeps fitting attachments and warns once when a batch exceeds the file count", async () => {
    const { result } = renderHook(() => useDialogAttachments(false), {
      wrapper: ({ children }) => React.createElement(ToastProvider, null, children),
    });
    const files = Array.from({ length: MAX_FILES + 1 }, (_, index) =>
      fileWithSize(`attachment-${index}.txt`, 1),
    );

    await act(async () => {
      await result.current.handleFileInputChange({
        target: { files, value: "" },
      } as unknown as React.ChangeEvent<HTMLInputElement>);
    });

    await waitFor(() => expect(result.current.attachments).toHaveLength(MAX_FILES));
    expect(result.current.attachments.map((attachment) => attachment.fileName)).toEqual(
      files.slice(0, MAX_FILES).map((file) => file.name),
    );
    expect(screen.getAllByTestId(toastMessageTestId)).toHaveLength(1);
    expect(screen.getByTestId(toastMessageTestId).textContent).toContain(
      `You can attach up to ${MAX_FILES} files.`,
    );
  });

  it("keeps fitting attachments and warns once when a batch exceeds total size", async () => {
    const { result } = renderHook(() => useDialogAttachments(false), {
      wrapper: ({ children }) => React.createElement(ToastProvider, null, children),
    });
    const fileSize = MAX_TOTAL_SIZE / 3 + 1;
    const files = [
      fileWithSize("first.txt", fileSize),
      fileWithSize("second.txt", fileSize),
      fileWithSize("third.txt", fileSize),
    ];

    await act(async () => {
      await result.current.handleFileInputChange({
        target: { files, value: "" },
      } as unknown as React.ChangeEvent<HTMLInputElement>);
    });

    await waitFor(() => expect(result.current.attachments).toHaveLength(2));
    expect(result.current.attachments.map((attachment) => attachment.fileName)).toEqual([
      "first.txt",
      "second.txt",
    ]);
    expect(screen.getAllByTestId(toastMessageTestId)).toHaveLength(1);
    expect(screen.getByTestId(toastMessageTestId).textContent).toContain(
      `Attachments can total up to ${formatBytes(MAX_TOTAL_SIZE)}.`,
    );
  });
});
