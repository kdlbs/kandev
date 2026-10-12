import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setChatDraftAttachments, setChatDraftText } from "@/lib/local-storage";

const deleteAttachment = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/attachment-api", () => ({
  deleteAttachment: (...args: unknown[]) => deleteAttachment(...args),
}));

import { useQuickChatSetupDraft } from "./use-quick-chat-setup-draft";

const WORKSPACE_ID = "workspace-1";
const USER_ID = "user-1";
const DRAFT_ID = `quick-chat-setup-draft:${USER_ID}:${WORKSPACE_ID}`;

beforeEach(() => {
  window.sessionStorage.clear();
  deleteAttachment.mockReset().mockResolvedValue(undefined);
});

afterEach(() => {
  window.sessionStorage.clear();
});

describe("useQuickChatSetupDraft", () => {
  it("restores text and staged attachments from the setup draft storage", () => {
    const attachment = {
      id: "stored-1",
      attachmentId: "attachment-1",
      mimeType: "text/plain",
      fileName: "notes.txt",
      size: 12,
      isImage: false,
      deliveryMode: "path" as const,
    };
    setChatDraftText(DRAFT_ID, "Review these notes");
    setChatDraftAttachments(DRAFT_ID, [attachment]);

    const { result } = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, USER_ID));

    expect(result.current.draft.message).toBe("Review these notes");
    expect(result.current.draft.attachments).toEqual([{ ...attachment, uploadStatus: "ready" }]);
  });

  it("retains explicit profile and repository choices across setup remounts", () => {
    const { result, unmount } = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, USER_ID));

    act(() => {
      result.current.update({
        message: "Keep this draft",
        agentProfileId: "agent-a",
        agentProfileExplicit: true,
        repositories: [{ key: "repo-row", repositoryId: "repo-1", branch: "feature/work" }],
      });
    });
    unmount();

    const remounted = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, USER_ID));

    expect(remounted.result.current.draft).toMatchObject({
      message: "Keep this draft",
      agentProfileId: "agent-a",
      agentProfileExplicit: true,
      repositories: [{ key: "repo-row", repositoryId: "repo-1", branch: "feature/work" }],
    });
  });

  it("deletes staged files only when the setup draft is explicitly discarded", () => {
    const { result } = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, USER_ID));
    act(() =>
      result.current.update({
        attachments: [
          {
            id: "attachment-row",
            attachmentId: "attachment-staged",
            mimeType: "image/png",
            fileName: "diagram.png",
            size: 8,
            isImage: true,
            deliveryMode: "prompt",
            uploadStatus: "ready",
          },
        ],
      }),
    );

    act(() => result.current.clear(false));
    expect(deleteAttachment).not.toHaveBeenCalled();

    act(() =>
      result.current.update({
        attachments: [
          {
            id: "attachment-row",
            attachmentId: "attachment-staged",
            mimeType: "image/png",
            fileName: "diagram.png",
            size: 8,
            isImage: true,
            deliveryMode: "prompt",
            uploadStatus: "ready",
          },
        ],
      }),
    );
    act(() => result.current.clear(true));

    expect(deleteAttachment).toHaveBeenCalledWith("attachment-staged");
    expect(result.current.draft.attachments).toEqual([]);
  });

  it("keeps the same workspace draft separate for each signed-in user", () => {
    const first = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, "user-a"));
    act(() => first.result.current.update({ message: "private draft" }));
    first.unmount();

    const second = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, "user-b"));
    expect(second.result.current.draft.message).toBe("");

    const firstAgain = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, "user-a"));
    expect(firstAgain.result.current.draft.message).toBe("private draft");
  });

  it("restores each workspace draft after switching the active workspace", () => {
    const view = renderHook(({ workspaceId }) => useQuickChatSetupDraft(workspaceId, USER_ID), {
      initialProps: { workspaceId: WORKSPACE_ID },
    });
    act(() => view.result.current.update({ message: "Workspace one" }));
    view.rerender({ workspaceId: "workspace-2" });
    act(() => view.result.current.update({ message: "Workspace two" }));

    view.rerender({ workspaceId: WORKSPACE_ID });
    expect(view.result.current.draft.message).toBe("Workspace one");
    view.rerender({ workspaceId: "workspace-2" });
    expect(view.result.current.draft.message).toBe("Workspace two");
  });
});

it("restores expired staged uploads as failed while preserving their expiry", () => {
  const expired = {
    id: "expired-row",
    attachmentId: "expired-attachment",
    expiresAt: "2020-01-01T00:00:00Z",
    mimeType: "text/plain",
    fileName: "expired.txt",
    size: 12,
    isImage: false,
    deliveryMode: "path" as const,
  };
  const active = {
    id: "active-row",
    attachmentId: "active-attachment",
    expiresAt: "2099-01-01T00:00:00Z",
    mimeType: "text/plain",
    fileName: "active.txt",
    size: 12,
    isImage: false,
    deliveryMode: "path" as const,
  };
  setChatDraftAttachments(DRAFT_ID, [expired, active]);

  const { result } = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, USER_ID));

  expect(result.current.draft.attachments).toEqual([
    expect.objectContaining({
      ...expired,
      uploadStatus: "failed",
      uploadError: expect.any(String),
    }),
    expect.objectContaining({ ...active, uploadStatus: "ready" }),
  ]);
});
