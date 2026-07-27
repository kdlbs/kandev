import { act, cleanup, renderHook, screen } from "@testing-library/react";
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import { MAX_FILE_SIZE } from "./chat/file-attachment";
import { useDialogAttachments } from "./session-dialog-shared";

afterEach(cleanup);

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
    expect(screen.getByTestId("toast-message").textContent).toContain(
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
    expect(screen.getByTestId("toast-message").textContent).toContain("Attachment is too large");
    expect(result.current.attachments).toEqual([]);
  });
});
