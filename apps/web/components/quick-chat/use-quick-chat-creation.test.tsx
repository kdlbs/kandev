import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { getQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import type { QuickChatOpeningPayload } from "@/lib/state/slices/ui/types";

const getCreation = vi.hoisted(() => vi.fn());
const deleteTask = vi.hoisted(() => vi.fn());
const handleSelectAgent = vi.hoisted(() => vi.fn());
const reportSetupError = vi.hoisted(() => vi.fn());
let appState: {
  openQuickChat: ReturnType<typeof vi.fn>;
  upsertQuickChatSessionFromEvent: ReturnType<typeof vi.fn>;
  setQuickChatInitialPrompt: ReturnType<typeof vi.fn>;
  agentProfiles: { items: Array<{ id: string; cli_passthrough?: boolean }> };
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof appState) => unknown) => selector(appState),
}));

vi.mock("@/lib/api/domains/workspace-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/workspace-api")>()),
  getQuickChatCreation: (...args: unknown[]) => getCreation(...args),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  deleteTaskAfterUserAction: (...args: unknown[]) => deleteTask(...args),
}));

import { useQuickChatCreation } from "./use-quick-chat-creation";
import { useQuickChatSetupDraft } from "./use-quick-chat-setup-draft";

const WORKSPACE_ID = "workspace-creation";
const USER_ID = "user-creation";
const SETUP_SESSION_ID = getQuickChatSetupSessionId(WORKSPACE_ID, "chat");
const OPENING_REQUEST = "Opening request";
const ACTIVATION_DRAFT = "Keep this until activation";
const CORRECTABLE_DRAFT = "Keep the draft editable";
const STABLE_MESSAGE_ID = "message-stable";
const UNCERTAIN_DRAFT = "Keep this uncertain draft";
const UPLOADED_ATTACHMENT_ID = "uploaded-id";
const ATTACHMENT_MIME_TYPE = "text/plain";
const ATTACHMENT_FILE_NAME = "notes.txt";

function makeQuickChat(isOpen = true) {
  return {
    isOpen,
    activeSessionId: isOpen ? SETUP_SESSION_ID : null,
    setupKey: 0,
    sessions: [],
    handleSelectAgent,
    reportSetupError,
  };
}

function useCreationHarness(quickChat = makeQuickChat()) {
  const setupDraft = useQuickChatSetupDraft(WORKSPACE_ID, USER_ID);
  const creation = useQuickChatCreation({
    workspaceId: WORKSPACE_ID,
    userId: USER_ID,
    quickChat,
    setupDraft,
  });
  return { setupDraft, creation, quickChat };
}

function openingPayload(message: string, clientMessageId = STABLE_MESSAGE_ID) {
  return { message, clientMessageId } satisfies QuickChatOpeningPayload;
}

beforeEach(() => {
  window.sessionStorage.clear();
  handleSelectAgent.mockReset().mockResolvedValue(false);
  reportSetupError.mockReset();
  getCreation.mockReset();
  deleteTask.mockReset().mockResolvedValue(undefined);
  appState = {
    openQuickChat: vi.fn(),
    upsertQuickChatSessionFromEvent: vi.fn(),
    setQuickChatInitialPrompt: vi.fn(),
    agentProfiles: { items: [{ id: "agent-1" }] },
  };
});

it("keeps an uncertain submission and retries with the same identity before clearing its draft", async () => {
  handleSelectAgent
    .mockImplementationOnce(async (...args: unknown[]) => {
      (args[4] as (error: unknown) => void)(new ApiError("gateway timeout", 502, null));
      return false;
    })
    .mockResolvedValueOnce(true);
  getCreation.mockRejectedValue(new Error("status unavailable"));
  const { result } = renderHook(() => useCreationHarness());
  act(() => result.current.setupDraft.update({ message: OPENING_REQUEST }));

  await act(async () => {
    await result.current.creation.start(
      "agent-1",
      [{ repository_id: "repo-1", base_branch: "main" }],
      openingPayload(OPENING_REQUEST),
      { message: OPENING_REQUEST, attachments: [] },
    );
  });
  const firstCall = handleSelectAgent.mock.calls[0] as unknown[];
  const pending = result.current.setupDraft.pendingCreation;
  expect(pending?.state).toBe("unknown");
  expect(pending?.creationId).toBe(firstCall[3]);
  expect(pending?.clientMessageId).toBe(STABLE_MESSAGE_ID);

  await act(async () => {
    expect(await result.current.creation.retry()).toBe(true);
  });
  const retryCall = handleSelectAgent.mock.calls[1] as unknown[];
  expect(retryCall[3]).toBe(firstCall[3]);
  expect((retryCall[2] as QuickChatOpeningPayload).clientMessageId).toBe(STABLE_MESSAGE_ID);
  expect(result.current.setupDraft.pendingCreation).toBeNull();
  expect(result.current.setupDraft.draft.message).toBe("");
});

it("clears an unchanged uploaded draft after same-token retry and a new setup mount", async () => {
  handleSelectAgent
    .mockImplementationOnce(async (...args: unknown[]) => {
      (args[4] as (error: unknown) => void)(new ApiError("gateway timeout", 502, null));
      return false;
    })
    .mockResolvedValueOnce(true);
  getCreation.mockRejectedValue(new Error("status unavailable"));
  const attachment = {
    id: "local-row-id",
    attachmentId: UPLOADED_ATTACHMENT_ID,
    mimeType: ATTACHMENT_MIME_TYPE,
    fileName: ATTACHMENT_FILE_NAME,
    size: 5,
    isImage: false,
    deliveryMode: "path" as const,
    uploadStatus: "ready" as const,
  };
  const { result, unmount } = renderHook(() => useCreationHarness());
  act(() =>
    result.current.setupDraft.update({ message: OPENING_REQUEST, attachments: [attachment] }),
  );

  await act(async () => {
    await result.current.creation.start(
      "agent-1",
      [],
      {
        ...openingPayload(OPENING_REQUEST),
        attachments: [
          {
            type: "resource",
            mime_type: ATTACHMENT_MIME_TYPE,
            name: ATTACHMENT_FILE_NAME,
            attachment_id: UPLOADED_ATTACHMENT_ID,
          },
        ],
      },
      { message: OPENING_REQUEST, attachments: [attachment] },
    );
  });
  const creationId = result.current.setupDraft.pendingCreation?.creationId;
  expect(creationId).toBeTruthy();
  expect(result.current.setupDraft.draft.attachments).toHaveLength(1);

  await act(async () => {
    expect(await result.current.creation.retry()).toBe(true);
  });
  expect(result.current.setupDraft.draft).toMatchObject({ message: "", attachments: [] });
  expect(result.current.setupDraft.pendingCreation).toBeNull();
  unmount();

  const newSetup = renderHook(() => useQuickChatSetupDraft(WORKSPACE_ID, USER_ID));
  expect(newSetup.result.current.draft).toMatchObject({ message: "", attachments: [] });
  expect(newSetup.result.current.pendingCreation).toBeNull();
});

it("checks a restored receipt without posting or replaying an attempted opening message", async () => {
  window.sessionStorage.setItem(
    `quick-chat-creation:${USER_ID}:${WORKSPACE_ID}`,
    JSON.stringify({
      creationId: "restored-creation",
      clientMessageId: "restored-message",
      message: "Already attempted opening",
      attachments: [],
      agentProfileId: "agent-1",
      repositories: [],
      runtimeId: "runtime-1",
      state: "unknown",
      messageAttempted: true,
    }),
  );
  getCreation.mockResolvedValue({
    task_id: "task-restored",
    session_id: "session-restored",
    creation_id: "restored-creation",
    creation_complete: true,
    agent_profile_id: "agent-1",
    session_state: "WAITING_FOR_INPUT",
  });

  renderHook(() => useCreationHarness());

  await waitFor(() => expect(getCreation).toHaveBeenCalledWith(WORKSPACE_ID, "restored-creation"));
  expect(handleSelectAgent).not.toHaveBeenCalled();
  expect(appState.setQuickChatInitialPrompt).not.toHaveBeenCalled();
  expect(appState.openQuickChat).toHaveBeenCalledWith(
    "session-restored",
    WORKSPACE_ID,
    "agent-1",
    "chat",
    "task-restored",
  );
});

it("accepts status with a stable uploaded attachment identity and clears only the unchanged draft", async () => {
  getCreation.mockImplementation(async (_workspaceId: string, creationId: string) => ({
    task_id: "task-1",
    session_id: "session-1",
    creation_id: creationId,
    creation_complete: true,
    agent_profile_id: "agent-1",
    session_state: "WAITING_FOR_INPUT",
  }));
  const { result } = renderHook(() => useCreationHarness());
  const attachment = {
    id: "local-upload-row",
    attachmentId: "uploaded-attachment",
    mimeType: ATTACHMENT_MIME_TYPE,
    fileName: ATTACHMENT_FILE_NAME,
    size: 9,
    isImage: false,
    deliveryMode: "path" as const,
    uploadStatus: "ready" as const,
  };
  act(() =>
    result.current.setupDraft.update({ message: "Captured opening", attachments: [attachment] }),
  );
  act(() => {
    result.current.setupDraft.beginPendingCreation(
      {
        ...openingPayload("Captured opening"),
        attachments: [
          {
            type: "resource",
            mime_type: ATTACHMENT_MIME_TYPE,
            name: ATTACHMENT_FILE_NAME,
            attachment_id: "uploaded-attachment",
          },
        ],
      },
      "agent-1",
      [],
    );
  });
  const pending = result.current.setupDraft.pendingCreation!;

  await act(async () => result.current.creation.check(pending));

  expect(appState.upsertQuickChatSessionFromEvent).toHaveBeenCalledWith(
    expect.objectContaining({ sessionId: "session-1", creationId: pending.creationId }),
  );
  expect(appState.openQuickChat).toHaveBeenCalledWith(
    "session-1",
    WORKSPACE_ID,
    "agent-1",
    "chat",
    "task-1",
  );
  expect(appState.setQuickChatInitialPrompt).toHaveBeenCalledWith(
    "session-1",
    expect.objectContaining({
      message: "Captured opening",
      clientMessageId: STABLE_MESSAGE_ID,
    }),
  );
  expect(result.current.setupDraft.draft).toMatchObject({ message: "", attachments: [] });
  expect(result.current.setupDraft.pendingCreation).toBeNull();
});

it("records accepted status without opening or clearing after setup dismissal", async () => {
  let resolveStatus!: (value: unknown) => void;
  getCreation.mockReturnValue(new Promise((resolve) => (resolveStatus = resolve)));
  const quickChat = makeQuickChat(true);
  const { result, rerender } = renderHook(
    ({ open, setupKey }) => {
      quickChat.isOpen = open;
      quickChat.activeSessionId = open ? SETUP_SESSION_ID : null;
      quickChat.setupKey = setupKey;
      return useCreationHarness(quickChat);
    },
    { initialProps: { open: true, setupKey: 0 } },
  );
  act(() => result.current.setupDraft.update({ message: ACTIVATION_DRAFT }));
  act(() => {
    result.current.setupDraft.beginPendingCreation(openingPayload(ACTIVATION_DRAFT), "agent-1", []);
  });
  const pending = result.current.setupDraft.pendingCreation!;
  let checking!: Promise<void>;
  act(() => {
    checking = result.current.creation.check(pending);
  });
  rerender({ open: false, setupKey: 0 });
  await act(async () => {
    resolveStatus({
      task_id: "task-late",
      session_id: "session-late",
      creation_id: pending.creationId,
      creation_complete: true,
      agent_profile_id: "agent-1",
    });
    await checking;
  });

  expect(appState.openQuickChat).not.toHaveBeenCalled();
  expect(result.current.setupDraft.pendingCreation).toMatchObject({ state: "accepted" });
  expect(result.current.setupDraft.draft.message).toBe(ACTIVATION_DRAFT);
});

it("does not let a delayed status check steal a reopened setup with the same session id", async () => {
  let resolveStatus!: (value: unknown) => void;
  getCreation.mockReturnValue(new Promise((resolve) => (resolveStatus = resolve)));
  const quickChat = makeQuickChat(true);
  const { result, rerender } = renderHook(
    ({ open, setupKey }) => {
      quickChat.isOpen = open;
      quickChat.activeSessionId = open ? SETUP_SESSION_ID : null;
      quickChat.setupKey = setupKey;
      return useCreationHarness(quickChat);
    },
    { initialProps: { open: true, setupKey: 0 } },
  );
  act(() => result.current.setupDraft.update({ message: ACTIVATION_DRAFT }));
  act(() => {
    result.current.setupDraft.beginPendingCreation(openingPayload(ACTIVATION_DRAFT), "agent-1", []);
  });
  const pending = result.current.setupDraft.pendingCreation!;
  let checking!: Promise<void>;
  act(() => {
    checking = result.current.creation.check(pending);
  });

  rerender({ open: false, setupKey: 0 });
  rerender({ open: true, setupKey: 1 });
  await act(async () => {
    resolveStatus({
      task_id: "task-late",
      session_id: "session-late",
      creation_id: pending.creationId,
      creation_complete: true,
      agent_profile_id: "agent-1",
    });
    await checking;
  });

  expect(appState.openQuickChat).not.toHaveBeenCalled();
  expect(appState.setQuickChatInitialPrompt).not.toHaveBeenCalled();
  expect(result.current.setupDraft.pendingCreation).toMatchObject({ state: "accepted" });
  expect(result.current.setupDraft.draft.message).toBe(ACTIVATION_DRAFT);
});

it("does not let a delayed status check steal selection after a tab change", async () => {
  let resolveStatus!: (value: unknown) => void;
  getCreation.mockReturnValue(new Promise((resolve) => (resolveStatus = resolve)));
  const quickChat = makeQuickChat(true);
  const { result, rerender } = renderHook(
    ({ activeSessionId }) => {
      quickChat.isOpen = true;
      quickChat.activeSessionId = activeSessionId;
      return useCreationHarness(quickChat);
    },
    { initialProps: { activeSessionId: SETUP_SESSION_ID } },
  );
  act(() => result.current.setupDraft.update({ message: ACTIVATION_DRAFT }));
  act(() => {
    result.current.setupDraft.beginPendingCreation(openingPayload(ACTIVATION_DRAFT), "agent-1", []);
  });
  const pending = result.current.setupDraft.pendingCreation!;
  let checking!: Promise<void>;
  act(() => {
    checking = result.current.creation.check(pending);
  });

  rerender({ activeSessionId: "another-session" });
  rerender({ activeSessionId: SETUP_SESSION_ID });
  await act(async () => {
    resolveStatus({
      task_id: "task-late",
      session_id: "session-late",
      creation_id: pending.creationId,
      creation_complete: true,
      agent_profile_id: "agent-1",
    });
    await checking;
  });

  expect(appState.openQuickChat).not.toHaveBeenCalled();
  expect(appState.setQuickChatInitialPrompt).not.toHaveBeenCalled();
  expect(result.current.setupDraft.pendingCreation).toMatchObject({ state: "accepted" });
  expect(result.current.setupDraft.draft.message).toBe(ACTIVATION_DRAFT);
});

it("retires only a definitive pre-allocation rejection and permits a corrected retry", async () => {
  handleSelectAgent
    .mockImplementationOnce(async (...args: unknown[]) => {
      (args[4] as (error: unknown) => void)(
        new ApiError("repository not found", 404, { error_code: "quick_chat_creation_rejected" }),
      );
      return false;
    })
    .mockResolvedValueOnce(true);
  const { result } = renderHook(() => useCreationHarness());
  act(() => result.current.setupDraft.update({ message: CORRECTABLE_DRAFT }));

  await act(async () => {
    await result.current.creation.start(
      "agent-1",
      [{ repository_id: "deleted-repo", base_branch: "main" }],
      openingPayload(CORRECTABLE_DRAFT),
      { message: CORRECTABLE_DRAFT, attachments: [] },
    );
  });
  expect(result.current.setupDraft.pendingCreation).toBeNull();
  expect(result.current.setupDraft.draft.message).toBe(CORRECTABLE_DRAFT);

  await act(async () => {
    expect(
      await result.current.creation.start(
        "agent-1",
        [{ repository_id: "corrected-repo", base_branch: "main" }],
        openingPayload(CORRECTABLE_DRAFT),
        { message: CORRECTABLE_DRAFT, attachments: [] },
      ),
    ).toBe(true);
  });
  const failedId = (handleSelectAgent.mock.calls[0] as unknown[])[3];
  const correctedCall = handleSelectAgent.mock.calls[1] as unknown[];
  expect(correctedCall[3]).not.toBe(failedId);
  expect(correctedCall[1]).toEqual([{ repository_id: "corrected-repo", base_branch: "main" }]);
});

it("keeps an untyped 404 uncertain because it does not prove allocation failed", async () => {
  handleSelectAgent.mockImplementationOnce(async (...args: unknown[]) => {
    (args[4] as (error: unknown) => void)(new ApiError("not found", 404, null));
    return false;
  });
  getCreation.mockRejectedValue(new ApiError("not found", 404, null));
  const { result } = renderHook(() => useCreationHarness());
  act(() => result.current.setupDraft.update({ message: UNCERTAIN_DRAFT }));

  await act(async () => {
    await result.current.creation.start("agent-1", [], openingPayload(UNCERTAIN_DRAFT), {
      message: UNCERTAIN_DRAFT,
      attachments: [],
    });
  });

  expect(result.current.setupDraft.pendingCreation?.state).toBe("unknown");
  expect(result.current.setupDraft.draft.message).toBe(UNCERTAIN_DRAFT);
  expect(handleSelectAgent).toHaveBeenCalledTimes(1);
});

it("deletes a confirmed task and retires its pending receipt", async () => {
  const { result } = renderHook(() => useCreationHarness());
  act(() => {
    result.current.setupDraft.beginPendingCreation(
      openingPayload("Delete this chat"),
      "agent-1",
      [],
    );
  });
  const pending = result.current.setupDraft.pendingCreation!;
  act(() =>
    result.current.setupDraft.updatePendingCreation(pending.creationId, { taskId: "task-delete" }),
  );

  await act(async () => result.current.creation.remove());

  expect(deleteTask).toHaveBeenCalledWith("task-delete");
  expect(result.current.setupDraft.pendingCreation).toBeNull();
});

it("keeps the receipt and reports a deletion failure", async () => {
  const { result } = renderHook(() => useCreationHarness());
  act(() => {
    result.current.setupDraft.beginPendingCreation(openingPayload("Keep this chat"), "agent-1", []);
  });
  const pending = result.current.setupDraft.pendingCreation!;
  act(() =>
    result.current.setupDraft.updatePendingCreation(pending.creationId, { taskId: "task-delete" }),
  );
  deleteTask.mockRejectedValueOnce(new ApiError("delete unavailable", 503, null));

  await act(async () => {
    await result.current.creation.remove();
  });

  expect(result.current.setupDraft.pendingCreation).toMatchObject({
    creationId: pending.creationId,
  });
  expect(reportSetupError).toHaveBeenCalledWith(expect.any(ApiError));
});
