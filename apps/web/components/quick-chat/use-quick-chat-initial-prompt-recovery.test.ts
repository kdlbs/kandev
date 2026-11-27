import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";
import { useQuickChatInitialPromptRecovery } from "./use-quick-chat-initial-prompt-recovery";

const SESSION_ID = "session-1";

describe("useQuickChatInitialPromptRecovery", () => {
  it("restores staged attachments from a rejected opening payload into the mounted input", () => {
    const attachments = [
      {
        type: "resource" as const,
        attachment_id: "staged-attachment-1",
        mime_type: "text/plain",
        name: "notes.txt",
        size_bytes: 12,
        delivery_mode: "path" as const,
      },
    ];
    const insertText = vi.fn();
    const restoreStagedAttachments = vi.fn();
    const chatInputRef = {
      current: {
        getValue: () => "",
        insertText,
        restoreStagedAttachments,
      } as unknown as ChatInputContainerHandle,
    };
    const { result } = renderHook(() =>
      useQuickChatInitialPromptRecovery(SESSION_ID, chatInputRef),
    );

    result.current.restoreRejectedPrompt(SESSION_ID, {
      message: "Review these notes",
      attachments,
    });

    expect(insertText).toHaveBeenCalledWith("Review these notes", 0, 0);
    expect(restoreStagedAttachments).toHaveBeenCalledWith(attachments);
  });

  it("does not restore staged attachments into a different session input", () => {
    const restoreStagedAttachments = vi.fn();
    const chatInputRef = {
      current: {
        getValue: () => "",
        insertText: vi.fn(),
        restoreStagedAttachments,
      } as unknown as ChatInputContainerHandle,
    };
    const { result } = renderHook(() =>
      useQuickChatInitialPromptRecovery(SESSION_ID, chatInputRef),
    );

    result.current.restoreRejectedPrompt("session-2", {
      message: "Review these notes",
      attachments: [
        {
          type: "resource",
          attachment_id: "staged-attachment-2",
          mime_type: "text/plain",
          name: "notes.txt",
        },
      ],
    });

    expect(restoreStagedAttachments).not.toHaveBeenCalled();
  });
});
