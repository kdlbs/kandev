import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useProcessedMessages } from "./use-processed-messages";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type MessageType,
} from "@/lib/types/http";

const TASK_DESCRIPTION = "task description";
const SUBMITTED_TEXT = "submitted text";

function makeMessage(id: string, authorType: "agent" | "user", content: string): Message {
  return {
    id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: authorType,
    content,
    type: "message" satisfies MessageType,
    created_at: "",
  };
}

describe("initial prompt preview rendering", () => {
  it("shows submitted attachments before the stored initial prompt arrives", () => {
    const attachments = [
      {
        attachment_id: "image-1",
        type: "image",
        mime_type: "image/png",
        name: "screen.png",
        size_bytes: 32,
      },
      {
        attachment_id: "file-1",
        type: "resource",
        mime_type: "text/plain",
        name: "notes.txt",
        size_bytes: 12,
      },
    ];
    const { result } = renderHook(() =>
      useProcessedMessages([], "t1", "s1", TASK_DESCRIPTION, {
        historyInitialized: true,
        hasOlderMessages: false,
        initialPromptPreview: { content: SUBMITTED_TEXT, attachments },
      }),
    );
    expect(result.current.allMessages[0]).toMatchObject({
      content: SUBMITTED_TEXT,
      metadata: { attachments },
    });
  });

  it("hydrates an attachment-only preview and replaces it with stored history", () => {
    const attachment = {
      attachment_id: "image-1",
      type: "image",
      mime_type: "image/png",
      name: "screen.png",
    };
    const { result, rerender } = renderHook(
      ({
        preview,
        messages,
        sessionId,
      }: {
        preview: unknown;
        messages: Message[];
        sessionId: string;
      }) =>
        useProcessedMessages(messages, "t1", sessionId, "", {
          historyInitialized: true,
          hasOlderMessages: false,
          initialPromptPreview: preview,
        }),
      {
        initialProps: { preview: undefined as unknown, messages: [] as Message[], sessionId: "s1" },
      },
    );
    expect(result.current.allMessages).toEqual([]);
    const preview = { content: "", attachments: [attachment] };
    rerender({ preview, messages: [], sessionId: "s1" });
    expect(result.current.allMessages[0]).toMatchObject({
      content: "",
      metadata: { attachments: [attachment] },
    });
    const stored = {
      ...makeMessage("stored", "user", ""),
      metadata: { attachments: [attachment] },
    };
    rerender({ preview, messages: [stored], sessionId: "s1" });
    expect(result.current.allMessages).toEqual([stored]);
    rerender({ preview: undefined, messages: [], sessionId: "s2" });
    expect(result.current.allMessages).toEqual([]);
  });
});

describe("initial prompt preview guards", () => {
  it.each([
    { historyInitialized: false, hasOlderMessages: false },
    { historyInitialized: true, hasOlderMessages: true },
  ])("does not bypass history guards with preview metadata: %j", (history) => {
    const { result } = renderHook(() =>
      useProcessedMessages([], "t1", "s1", TASK_DESCRIPTION, {
        ...history,
        initialPromptPreview: { content: SUBMITTED_TEXT, attachments: [] },
      }),
    );
    expect(result.current.allMessages).toEqual([]);
  });

  it("discards malformed preview entries independently and strips non-display data", () => {
    const attachment = {
      attachment_id: "image-1",
      type: "image",
      mime_type: "image/png",
      name: "screen.png",
    };
    const { result } = renderHook(() =>
      useProcessedMessages([], "t1", "s1", TASK_DESCRIPTION, {
        historyInitialized: true,
        hasOlderMessages: false,
        initialPromptPreview: {
          content: SUBMITTED_TEXT,
          attachments: [
            null,
            "invalid",
            {},
            { ...attachment, data: "private", storage_key: "private/path" },
            { ...attachment, type: "audio" },
          ],
        },
      }),
    );
    expect(result.current.allMessages[0].metadata).toEqual({ attachments: [attachment] });
  });
});
