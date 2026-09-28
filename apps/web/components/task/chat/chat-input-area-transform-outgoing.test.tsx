import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";

const toastMock = vi.fn();
const handleSendMessageMock = vi.fn();

const mockState = {
  userSettings: { keyboardShortcuts: {}, chatSubmitKey: "enter" },
  quickChat: { sessions: [] },
  kanban: { workflowId: null, tasks: [], steps: [] },
  kanbanMulti: { snapshots: {} },
  workflows: { items: [] },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

vi.mock("@/hooks/use-message-handler", () => ({
  buildTaskMentionsContext: vi.fn(),
  useMessageHandler: () => ({ handleSendMessage: handleSendMessageMock }),
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
    previewFeedback: [],
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
    planCommentMigration: {
      status: "complete",
      pendingCount: 0,
      failure: null,
      needsAttention: false,
      isReady: true,
      isBlocking: false,
      retry: vi.fn(),
    },
    ...overrides,
  } as never;
}

describe("useSubmitHandler transformOutgoing", () => {
  it("applies the transform to the onSend path before building the final message", async () => {
    const onSend = vi.fn();
    const transformOutgoing = vi.fn((message: string) => `About t-1: ${message}`);
    const { result } = renderHook(() =>
      useSubmitHandler(panelState(), onSend, { transformOutgoing }),
    );

    await act(async () => {
      await result.current.handleSubmit({ message: "why is this here" });
    });

    expect(transformOutgoing).toHaveBeenCalledWith("why is this here");
    expect(onSend).toHaveBeenCalledWith({ message: "About t-1: why is this here" });
  });

  it("applies the transform to the direct handleSendMessage path", async () => {
    const transformOutgoing = vi.fn((message: string) => `About t-1: ${message}`);
    const { result } = renderHook(() =>
      useSubmitHandler(panelState(), undefined, { transformOutgoing }),
    );

    await act(async () => {
      await result.current.handleSubmit({ message: "why is this here" });
    });

    expect(handleSendMessageMock).toHaveBeenCalledWith({
      message: "About t-1: why is this here",
    });
  });

  it("leaves the task chat submit unchanged with no transform", async () => {
    const { result } = renderHook(() => useSubmitHandler(panelState()));

    await act(async () => {
      await result.current.handleSubmit({ message: "plain message" });
    });

    expect(handleSendMessageMock).toHaveBeenCalledWith({ message: "plain message" });
  });

  it("shows the send-error toast and sends nothing when the transform throws", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const transformOutgoing = vi.fn(() => {
      throw new Error("transform failed");
    });
    const { result } = renderHook(() =>
      useSubmitHandler(panelState(), undefined, { transformOutgoing }),
    );

    await act(async () => {
      await expect(result.current.handleSubmit({ message: "hello" })).resolves.toBe(false);
    });

    expect(handleSendMessageMock).not.toHaveBeenCalled();
    expect(toastMock).toHaveBeenCalledWith({
      title: "Message send status unknown",
      description:
        "The connection dropped or timed out. Refresh the task to confirm whether it went through.",
      variant: "error",
    });
  });
});
