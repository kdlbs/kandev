import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";

const toastMock = vi.fn();
const handleSendMessageMock = vi.fn();

const mockState = {};

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

vi.mock("@/components/azure-devops/azure-devops-task-pull-request-chip", () => ({
  AzureDevOpsTaskPullRequestChip: () => null,
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
  useKeyboardShortcut: () => undefined,
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

import { useSubmitHandler } from "./chat-input-area";

beforeEach(() => {
  handleSendMessageMock.mockReset();
  handleSendMessageMock.mockResolvedValue(undefined);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("useSubmitHandler", () => {
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

  it("shows a toast when sending fails", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    handleSendMessageMock.mockRejectedValueOnce(new Error("WebSocket request timed out"));
    const { result } = renderHook(() => useSubmitHandler(panelState()));

    await act(async () => {
      await result.current.handleSubmit("hello");
    });

    expect(toastMock).toHaveBeenCalledWith({
      title: "Message send status unknown",
      description:
        "The connection dropped or timed out. Refresh the task to confirm whether it went through.",
      variant: "error",
    });
  });

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
      await result.current.handleSubmit("hello");
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
      await result.current.handleSubmit("hello");
    });

    const [sentMessage] = handleSendMessageMock.mock.calls[0] as [string];
    expect(sentMessage.match(/### Agent Message Comments/g)).toHaveLength(1);
    expect(sentMessage).toContain("> settled answer");
    expect(sentMessage).toContain("> Please expand this.");
    expect(markCommentsSent).toHaveBeenCalledWith(["comment-1"]);
  });
});
