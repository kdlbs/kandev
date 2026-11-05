import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { setChatDraftAttachments, setChatDraftText } from "@/lib/local-storage";

const deleteAttachment = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/attachment-api", () => ({
  deleteAttachment: (...args: unknown[]) => deleteAttachment(...args),
}));

import { useQuickChatSetupDraft } from "./use-quick-chat-setup-draft";

const WORKSPACE_ID = "workspace-1";
const USER_ID = "user-1";
const DRAFT_ID = `quick-chat-setup-draft:${USER_ID}:${WORKSPACE_ID}`;
const CAPTURED_MESSAGE = "Captured message";
const STORED_ATTACHMENT_ID = "attachment-1";
const TEXT_MIME_TYPE = "text/plain";

function pendingStorageKey(workspaceId: string, userId: string): string {
  return `quick-chat-creation:${userId}:${workspaceId}`;
}

beforeEach(() => {
  window.sessionStorage.clear();
  deleteAttachment.mockReset().mockResolvedValue(undefined);
});

afterEach(() => {
  window.sessionStorage.clear();
});

it("restores text and staged attachments from the setup draft storage", () => {
  const attachment = {
    id: "stored-1",
    attachmentId: STORED_ATTACHMENT_ID,
    mimeType: TEXT_MIME_TYPE,
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

it.each([
  {
    name: "workspace",
    initial: { workspaceId: WORKSPACE_ID, userId: USER_ID },
    next: { workspaceId: "workspace-2", userId: USER_ID },
  },
  {
    name: "auth",
    initial: { workspaceId: WORKSPACE_ID, userId: "user-a" },
    next: { workspaceId: WORKSPACE_ID, userId: "user-b" },
  },
])("hydrates a stored pending receipt when switching $name scope", ({ initial, next }) => {
  const pending = {
    creationId: "saved-creation",
    clientMessageId: "saved-message",
    message: "Pending opening",
    attachments: [],
    agentProfileId: "agent-a",
    repositories: [],
    runtimeId: "runtime-1",
    state: "unknown",
    messageAttempted: false,
  };
  window.sessionStorage.setItem(
    pendingStorageKey(next.workspaceId, next.userId),
    JSON.stringify(pending),
  );

  const view = renderHook(
    ({ workspaceId, userId }) => useQuickChatSetupDraft(workspaceId, userId),
    { initialProps: initial },
  );
  view.rerender(next);

  expect(view.result.current.pendingCreation).toMatchObject({
    creationId: "saved-creation",
    clientMessageId: "saved-message",
  });
  expect(
    JSON.parse(
      window.sessionStorage.getItem(pendingStorageKey(next.workspaceId, next.userId)) ?? "null",
    ),
  ).toMatchObject({ creationId: "saved-creation" });
});

it("persists one descriptor-only creation receipt and preserves newer edits", () => {
  const { result } = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, USER_ID));
  const attachment = {
    id: "attachment-row",
    attachmentId: STORED_ATTACHMENT_ID,
    mimeType: TEXT_MIME_TYPE,
    fileName: "notes.txt",
    size: 7,
    isImage: false,
    deliveryMode: "path" as const,
    uploadStatus: "ready" as const,
  };
  act(() =>
    result.current.update({
      message: CAPTURED_MESSAGE,
      attachments: [attachment],
      agentProfileId: "agent-1",
    }),
  );
  let pending!: ReturnType<typeof result.current.beginPendingCreation>;
  act(() => {
    pending = result.current.beginPendingCreation(
      {
        message: CAPTURED_MESSAGE,
        clientMessageId: "stable-message-id",
        attachments: [
          {
            type: "resource",
            mime_type: TEXT_MIME_TYPE,
            name: "notes.txt",
            attachment_id: STORED_ATTACHMENT_ID,
            data: "private-file-bytes",
          },
        ],
      },
      "agent-1",
      [{ repository_id: "repo-1", base_branch: "main" }],
    );
  });
  if (!pending) throw new Error("pending Quick Chat receipt was not created");

  const serialized = window.sessionStorage.getItem(pendingStorageKey(WORKSPACE_ID, USER_ID));
  expect(serialized).not.toContain("private-file-bytes");
  expect(serialized).toContain("stable-message-id");
  expect(pending.attachments).toEqual([
    expect.objectContaining({ attachment_id: STORED_ATTACHMENT_ID }),
  ]);

  act(() => result.current.update({ message: "Newer message" }));
  act(() => result.current.clearCapturedDraft(CAPTURED_MESSAGE, [attachment]));
  expect(result.current.draft.message).toBe("Newer message");

  const remounted = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, USER_ID));
  expect(remounted.result.current.pendingCreation).toMatchObject({
    creationId: pending.creationId,
    clientMessageId: "stable-message-id",
    message: CAPTURED_MESSAGE,
  });
});

it("clears the accepted upload snapshot when the restored row id differs", () => {
  const { result } = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, USER_ID));
  act(() =>
    result.current.update({
      message: "Accepted prompt",
      attachments: [
        {
          id: "local-row-id",
          attachmentId: "uploaded-attachment-id",
          mimeType: TEXT_MIME_TYPE,
          fileName: "notes.txt",
          size: 12,
          isImage: false,
          deliveryMode: "path",
          uploadStatus: "ready",
        },
      ],
    }),
  );

  act(() =>
    result.current.clearCapturedDraft("Accepted prompt", [
      {
        id: "uploaded-attachment-id",
        attachmentId: "uploaded-attachment-id",
        mimeType: TEXT_MIME_TYPE,
        fileName: "notes.txt",
      },
    ]),
  );

  expect(result.current.draft.message).toBe("");
  expect(result.current.draft.attachments).toEqual([]);
});

it("does not replace an unresolved pending creation", () => {
  const { result } = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, USER_ID));
  const payload = {
    message: "Original prompt",
    clientMessageId: "original-message-id",
    attachments: [],
  };
  let original!: ReturnType<typeof result.current.beginPendingCreation>;
  act(() => {
    original = result.current.beginPendingCreation(payload, "agent-a", []);
  });
  if (!original) throw new Error("pending Quick Chat receipt was not created");
  const originalCreationId = original.creationId;
  act(() => result.current.updatePendingCreation(originalCreationId, { state: "unknown" }));

  let replacement!: ReturnType<typeof result.current.beginPendingCreation>;
  act(() => {
    replacement = result.current.beginPendingCreation(
      { ...payload, message: "Replacement prompt", clientMessageId: "replacement-message-id" },
      "agent-a",
      [],
    );
  });

  expect(replacement).toBeNull();
  expect(result.current.pendingCreation).toMatchObject({
    creationId: original.creationId,
    clientMessageId: "original-message-id",
    message: "Original prompt",
    state: "unknown",
  });
});

it("restores expired staged uploads as failed while preserving their expiry", () => {
  const expired = {
    id: "expired-row",
    attachmentId: "expired-attachment",
    expiresAt: "2020-01-01T00:00:00Z",
    mimeType: TEXT_MIME_TYPE,
    fileName: "expired.txt",
    size: 12,
    isImage: false,
    deliveryMode: "path" as const,
  };
  const active = {
    id: "active-row",
    attachmentId: "active-attachment",
    expiresAt: "2099-01-01T00:00:00Z",
    mimeType: TEXT_MIME_TYPE,
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
