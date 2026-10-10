import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { ToastProvider } from "@/components/toast-provider";
import { getChatDraftAttachments, setChatDraftAttachments } from "@/lib/local-storage";
import { useChatInputState } from "./use-chat-input-state";
import type { TipTapInputHandle } from "./tiptap-input";

const uploadAttachmentMock = vi.hoisted(() => vi.fn());
const deleteAttachmentMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/attachment-api", () => ({
  uploadAttachment: uploadAttachmentMock,
  deleteAttachment: deleteAttachmentMock,
}));

const TEXT_MIME_TYPE = "text/plain";
const WORKSPACE_ID = "workspace-1";
const ATTACHMENT_ID = "uploaded-attachment";
const STAGED_ATTACHMENT_ID = "staged-attachment";
type SubmitHandler = Parameters<typeof useChatInputState>[0]["onSubmit"];

function renderInputState(onSubmit: SubmitHandler, workspaceId?: string) {
  return renderHook(
    () =>
      useChatInputState({
        sessionId: "session-1",
        workspaceId,
        isSending: false,
        contextItems: [],
        showRequestChangesTooltip: false,
        onSubmit,
      }),
    { wrapper: ({ children }) => React.createElement(ToastProvider, null, children) },
  );
}

function attachClearHandle(inputRef: React.RefObject<TipTapInputHandle | null>, clear: () => void) {
  (inputRef as React.MutableRefObject<Partial<TipTapInputHandle> | null>).current = {
    clear,
    getMentions: () => [],
    getTaskMentions: () => [],
    getEntityReferences: () => [],
  };
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  uploadAttachmentMock.mockReset().mockResolvedValue({
    attachment_id: ATTACHMENT_ID,
    name: "notes.txt",
    mime_type: TEXT_MIME_TYPE,
    kind: "resource",
    delivery_mode: "path",
    size_bytes: 7,
  });
  deleteAttachmentMock.mockReset().mockResolvedValue(undefined);
});

afterEach(cleanup);

describe("useChatInputState accepted opening payload", () => {
  it("restores rejected staged attachment descriptors into the mounted composer", async () => {
    const stagedAttachment = {
      id: STAGED_ATTACHMENT_ID,
      attachmentId: STAGED_ATTACHMENT_ID,
      expiresAt: "2030-01-01T00:00:00.000Z",
      mimeType: TEXT_MIME_TYPE,
      fileName: "notes.txt",
      size: 7,
      isImage: false,
      deliveryMode: "path" as const,
    };
    const { result } = renderInputState(vi.fn(), WORKSPACE_ID);

    act(() => {
      setChatDraftAttachments("session-1", [stagedAttachment]);
      result.current.restoreStagedAttachments([
        {
          type: "resource",
          attachment_id: STAGED_ATTACHMENT_ID,
          mime_type: TEXT_MIME_TYPE,
          name: "notes.txt",
          size_bytes: 7,
          delivery_mode: "path",
        },
      ]);
    });

    await waitFor(() => expect(result.current.attachments).toHaveLength(1));
    expect(result.current.attachments[0]).toEqual(
      expect.objectContaining({
        attachmentId: STAGED_ATTACHMENT_ID,
        fileName: "notes.txt",
        uploadStatus: "ready",
        expiresAt: stagedAttachment.expiresAt,
      }),
    );
    expect(getChatDraftAttachments("session-1")).toEqual([
      expect.objectContaining({ attachmentId: STAGED_ATTACHMENT_ID }),
    ]);
  });

  it("retains upload expiration metadata on a successfully staged attachment", async () => {
    const expiresAt = "2030-01-01T00:00:00.000Z";
    uploadAttachmentMock.mockResolvedValueOnce({
      attachment_id: ATTACHMENT_ID,
      name: "notes.txt",
      mime_type: TEXT_MIME_TYPE,
      kind: "resource",
      delivery_mode: "path",
      size_bytes: 7,
      expires_at: expiresAt,
    });
    const { result } = renderInputState(vi.fn(), WORKSPACE_ID);

    await act(async () => {
      await result.current.addFiles([new File(["notes"], "notes.txt", { type: TEXT_MIME_TYPE })]);
    });

    await waitFor(() => expect(result.current.attachments[0]?.attachmentId).toBe(ATTACHMENT_ID));
    expect(result.current.attachments[0]?.expiresAt).toBe(expiresAt);
  });

  it("clears the accepted editor text and attachment snapshot", async () => {
    const clear = vi.fn();
    const resetHeight = vi.fn();
    const { result } = renderInputState(vi.fn(), WORKSPACE_ID);

    await act(async () => {
      await result.current.addFiles([new File(["notes"], "notes.txt", { type: TEXT_MIME_TYPE })]);
    });
    await waitFor(() => expect(result.current.attachments[0]?.attachmentId).toBe(ATTACHMENT_ID));
    act(() => {
      result.current.handleChange("review these notes");
      attachClearHandle(result.current.inputRef, clear);
    });
    const payload = { message: "review these notes", attachments: result.current.getAttachments() };

    let cleared = false;
    act(() => {
      cleared = result.current.clearAcceptedPayload(payload, resetHeight);
    });

    expect(cleared).toBe(true);
    expect(clear).toHaveBeenCalledOnce();
    expect(resetHeight).toHaveBeenCalledOnce();
    await waitFor(() => {
      expect(result.current.value).toBe("");
      expect(result.current.attachments).toEqual([]);
    });
  });

  it("preserves newer draft text when the opening snapshot is accepted", () => {
    const resetHeight = vi.fn();
    const { result } = renderInputState(vi.fn());
    act(() => result.current.handleChange("new follow-up"));

    let cleared = true;
    act(() => {
      cleared = result.current.clearAcceptedPayload(
        { message: "opening request", attachments: [] },
        resetHeight,
      );
    });

    expect(cleared).toBe(false);
    expect(result.current.value).toBe("new follow-up");
    expect(resetHeight).not.toHaveBeenCalled();
  });
});
