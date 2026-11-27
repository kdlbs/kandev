import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";

const deleteAttachment = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/attachment-api", () => ({
  deleteAttachment: (...args: unknown[]) => deleteAttachment(...args),
}));

import { useQuickChatSetupDraft } from "./use-quick-chat-setup-draft";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  deleteAttachment.mockReset().mockResolvedValue(undefined);
});

it("rejects a delayed attachment update from a discarded setup generation", () => {
  const { result } = renderHook(() => useQuickChatSetupDraft("workspace-1", "user-1"));
  const lateUpdate = result.current.update;
  const attachment = {
    id: "late-row",
    attachmentId: "late-upload",
    mimeType: "text/plain",
    fileName: "late.txt",
    size: 8,
    isImage: false,
    deliveryMode: "path" as const,
    uploadStatus: "ready" as const,
  };

  act(() => result.current.clear(true));
  let accepted: boolean | void = undefined;
  act(() => {
    accepted = lateUpdate({ attachments: [attachment] });
  });

  expect(accepted).toBe(false);
  expect(result.current.draft.attachments).toEqual([]);
});
