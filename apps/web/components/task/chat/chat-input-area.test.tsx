import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";

const toastMock = vi.fn();
const handleSendMessageMock = vi.fn();
const useKeyboardShortcutMock = vi.hoisted(() => vi.fn());

const mockState = { userSettings: { keyboardShortcuts: {} } };

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

vi.mock("@/components/github/pr-status-chip", () => ({
  PRStatusChip: () => null,
}));

vi.mock("@/components/gitlab/mr-status-chip", () => ({
  MRStatusChip: () => null,
}));

vi.mock("@/components/azure-devops/azure-devops-task-pull-request-chip", () => ({
  AzureDevOpsTaskPullRequestChip: () => null,
}));

vi.mock("@/components/integrations/registered-change-request-status", () => ({
  RegisteredChangeRequestStatus: () => null,
}));

vi.mock("@/components/task/share/share-button", () => ({
  ShareButton: () => null,
  shareableSessionStateClient: () => false,
}));

vi.mock("@/components/task/chat/chat-input-container", () => ({
  ChatInputContainer: () => null,
}));

vi.mock("@/components/task/chat/todo-indicator", () => ({
  TodoIndicator: () => null,
}));

vi.mock("./pr-archive-banners", () => ({
  PRMergedBanner: () => null,
  PRClosedBanner: () => null,
}));

vi.mock("@/hooks/use-keyboard-shortcut", () => ({
  useKeyboardShortcut: useKeyboardShortcutMock,
}));

vi.mock("@/hooks/use-message-handler", () => ({
  buildTaskMentionsContext: vi.fn(),
  useMessageHandler: () => ({ handleSendMessage: handleSendMessageMock }),
}));

vi.mock("@/hooks/domains/kanban/use-plan-actions", () => ({
  usePlanActions: () => ({
    implementPlanHandler: vi.fn(),
    proceedStepName: null,
    proceed: vi.fn(),
    isMoving: false,
  }),
}));

vi.mock("@/hooks/domains/session/use-executor-environment-availability", () => ({
  useExecutorEnvironmentAvailability: () => ({
    unavailable: false,
    status: null,
  }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ send: vi.fn() }),
}));

import { resolveInputPlaceholder, useChatPanelHandlers, useSubmitHandler } from "./chat-input-area";

beforeEach(() => {
  handleSendMessageMock.mockReset();
  handleSendMessageMock.mockResolvedValue(undefined);
  useKeyboardShortcutMock.mockReset();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

function panelState(overrides = {}) {
  return {
    resolvedSessionId: "session-1",
    taskId: "task-1",
    sessionModel: null,
    activeModel: null,
    isAgentBusy: false,
    activeDocument: null,
    planComments: [],
    pendingPRFeedback: [],
    walkthroughComments: [],
    messageComments: [],
    contextFiles: [],
    prompts: [],
    markCommentsSent: vi.fn(),
    clearSessionPlanComments: vi.fn(),
    handleClearPRFeedback: vi.fn(),
    handleClearWalkthroughComments: vi.fn(),
    clearEphemeral: vi.fn(),
    addContextFile: vi.fn(),
    planModeEnabled: false,
    ...overrides,
  } as never;
}

describe("resolveInputPlaceholder", () => {
  it("invites queueing while a clarification remains pending", () => {
    expect(resolveInputPlaceholder(false, undefined, false, true, false)).toBe(
      "Queue instructions while the question is pending...",
    );
  });
});

describe("useSubmitHandler", () => {
  it("shows a toast when sending fails", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    handleSendMessageMock.mockRejectedValueOnce(new Error("WebSocket request timed out"));
    const { result } = renderHook(() => useSubmitHandler(panelState()));

    await act(async () => {
      await result.current.handleSubmit({ message: "hello" });
    });

    expect(toastMock).toHaveBeenCalledWith({
      title: "Message send status unknown",
      description:
        "The connection dropped or timed out. Refresh the task to confirm whether it went through.",
      variant: "error",
    });
  });
});

describe("useChatPanelHandlers", () => {
  it("can disable the global focus shortcut for embedded panels", () => {
    renderHook(() =>
      useChatPanelHandlers("session-1", { current: null }, { enableFocusShortcut: false }),
    );

    expect(useKeyboardShortcutMock).toHaveBeenCalledWith(
      expect.anything(),
      expect.anything(),
      expect.objectContaining({ enabled: false }),
    );
  });
});

describe("useSubmitHandler routing", () => {
  it("queues through the shared handler instead of preview onSend while clarification is pending", async () => {
    const onSend = vi.fn();
    const { result } = renderHook(() =>
      useSubmitHandler(panelState({ pendingClarification: { id: "clarification-1" } }), onSend),
    );

    await act(async () => {
      await result.current.handleSubmit({ message: "queued instruction" });
    });

    expect(onSend).not.toHaveBeenCalled();
    expect(handleSendMessageMock).toHaveBeenCalledWith({ message: "queued instruction" });
  });

  it("uses preview onSend when no clarification is pending", async () => {
    const onSend = vi.fn();
    const { result } = renderHook(() => useSubmitHandler(panelState(), onSend));

    await act(async () => {
      await result.current.handleSubmit({ message: "direct preview message" });
    });

    expect(onSend).toHaveBeenCalledWith({ message: "direct preview message" });
    expect(handleSendMessageMock).not.toHaveBeenCalled();
  });
});

describe("useSubmitHandler plan mode", () => {
  it("uses the normal send path in plan mode", async () => {
    const onSend = vi.fn();
    const { result } = renderHook(() =>
      useSubmitHandler(panelState({ planModeEnabled: true }), onSend),
    );

    await act(async () => {
      await result.current.handleSubmit({ message: "plan-mode instruction" });
    });

    expect(onSend).toHaveBeenCalledWith({ message: "plan-mode instruction" });
    expect(handleSendMessageMock).not.toHaveBeenCalled();
    expect(toastMock).not.toHaveBeenCalled();
  });
});

describe("useSubmitHandler deterministic failures", () => {
  it.each([
    ["connection-unavailable", "Connection unavailable. Reconnect and try again."],
    ["session-unavailable", "The selected session is not available for input."],
  ])("reports deterministic %s preflight failures as not sent", async (code, message) => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const error = Object.assign(new Error(message), {
      name: "MessageSendError",
      code,
    });
    handleSendMessageMock.mockRejectedValueOnce(error);
    const { result } = renderHook(() => useSubmitHandler(panelState()));

    await act(async () => {
      await result.current.handleSubmit({ message: "hello" });
    });

    expect(toastMock).toHaveBeenCalledWith({
      title: "Message not sent",
      description: message,
      variant: "error",
    });
  });
});

describe("useSubmitHandler message comments", () => {
  it("keeps agent message comments pending when sending fails", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const markCommentsSent = vi.fn();
    handleSendMessageMock.mockRejectedValueOnce(new Error("send failed"));
    const { result } = renderHook(() =>
      useSubmitHandler(
        panelState({
          messageComments: [{ id: "comment-1" }],
          markCommentsSent,
        }),
      ),
    );

    await act(async () => {
      await result.current.handleSubmit({ message: "hello" });
    });

    expect(markCommentsSent).not.toHaveBeenCalled();
  });

  it("includes each agent message comment once and marks it sent after success", async () => {
    const markCommentsSent = vi.fn();
    const comment = {
      id: "comment-1",
      selectedText: "settled answer",
      text: "Please expand this.",
    };
    const { result } = renderHook(() =>
      useSubmitHandler(
        panelState({
          messageComments: [comment],
          markCommentsSent,
        }),
      ),
    );

    await act(async () => {
      await result.current.handleSubmit({ message: "hello" });
    });

    const [{ message: sentMessage }] = handleSendMessageMock.mock.calls[0] as [{ message: string }];
    expect(sentMessage.match(/### Agent Message Comments/g)).toHaveLength(1);
    expect(sentMessage).toContain("> settled answer");
    expect(sentMessage).toContain("> Please expand this.");
    expect(markCommentsSent).toHaveBeenCalledWith(["comment-1"]);
  });
});

describe("context file send retention", () => {
  it("keeps ephemeral context files when sending fails", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const clearEphemeral = vi.fn();
    handleSendMessageMock.mockRejectedValueOnce(new Error("send failed"));
    const { result } = renderHook(() =>
      useSubmitHandler(
        panelState({
          contextFiles: [{ path: "src", name: "src", isDirectory: true }],
          clearEphemeral,
        }),
      ),
    );

    await act(async () => {
      await result.current.handleSubmit({ message: "hello" });
    });

    expect(clearEphemeral).not.toHaveBeenCalled();
  });

  it("clears ephemeral context files after a successful send", async () => {
    const clearEphemeral = vi.fn();
    handleSendMessageMock.mockResolvedValueOnce(undefined);
    const { result } = renderHook(() =>
      useSubmitHandler(
        panelState({
          contextFiles: [{ path: "src", name: "src", isDirectory: true }],
          clearEphemeral,
        }),
      ),
    );

    await act(async () => {
      await result.current.handleSubmit({ message: "hello" });
    });

    expect(clearEphemeral).toHaveBeenCalledWith("session-1");
  });
});
