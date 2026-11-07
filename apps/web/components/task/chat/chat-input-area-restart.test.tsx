import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useSessionRestart } from "@/hooks/domains/session/use-session-restart";
import { useSubmitHandler } from "./chat-input-area";

const SESSION_ID = "session-1";
const TASK_ID = "task-1";
const restartRequestMock = vi.fn();
const handleSendMessageMock = vi.fn();
const mockState = {
  tasks: { activeTaskId: TASK_ID, activeSessionId: null },
  taskRemoval: { navigationRevision: 0 },
  taskSessionsByTask: {
    itemsByTaskId: { [TASK_ID]: [{ id: SESSION_ID, state: "WAITING_FOR_INPUT" }] },
  },
};
const mockStoreApi = { getState: () => mockState };
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: typeof mockState) => unknown) => select(mockState),
  useAppStoreApi: () => mockStoreApi,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn(), updateToast: vi.fn() }),
}));
vi.mock("@/hooks/use-message-handler", () => ({
  buildTaskMentionsContext: vi.fn(),
  useMessageHandler: () => ({ handleSendMessage: handleSendMessageMock }),
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: restartRequestMock }),
}));
beforeEach(() => {
  vi.clearAllMocks();
  restartRequestMock.mockReset().mockResolvedValue(undefined);
  handleSendMessageMock.mockResolvedValue(undefined);
});

/** Builds the composer fixture with a live target and overridable recovery state. */
function panelState(overrides = {}) {
  return {
    resolvedSessionId: SESSION_ID,
    taskId: TASK_ID,
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
    consumeSubmittedEphemeral: vi.fn(),
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

describe("composer restart submission guard", () => {
  // @covers AC-TASKS-SESSION-TAB-RESTART-001.6
  it("blocks an earlier submit callback through Stop and Resume without blocking a sibling", async () => {
    let finishStop!: () => void;
    let finishResume!: () => void;
    const stop = new Promise<void>((resolve) => {
      finishStop = resolve;
    });
    const resume = new Promise<void>((resolve) => {
      finishResume = resolve;
    });
    restartRequestMock.mockImplementationOnce(() => stop).mockImplementationOnce(() => resume);
    const consumeSubmittedEphemeral = vi.fn();
    const onSend = vi.fn().mockResolvedValue(true);
    const { result } = renderHook(() => ({
      restart: useSessionRestart(SESSION_ID, TASK_ID),
      submit: useSubmitHandler(panelState({ consumeSubmittedEphemeral }), onSend),
      sibling: useSubmitHandler(panelState({ resolvedSessionId: "session-2" })),
    }));
    const submitBeforeRestart = result.current.submit.handleSubmit;
    let restarting!: Promise<boolean>;
    act(() => {
      restarting = result.current.restart.restart();
    });
    try {
      await act(async () => {
        expect(await submitBeforeRestart({ message: "keep this draft" })).toBe(false);
      });
      expect(onSend).not.toHaveBeenCalled();
      expect(consumeSubmittedEphemeral).not.toHaveBeenCalled();
      await act(async () => {
        finishStop();
        await stop;
      });
      expect(restartRequestMock).toHaveBeenLastCalledWith(
        "session.launch",
        { task_id: TASK_ID, intent: "resume", session_id: SESSION_ID },
        30000,
      );
      await act(async () => {
        expect(await submitBeforeRestart({ message: "still keep this draft" })).toBe(false);
        expect(await result.current.sibling.handleSubmit({ message: "sibling can send" })).toBe(
          true,
        );
      });
      expect(onSend).not.toHaveBeenCalled();
      expect(handleSendMessageMock).toHaveBeenCalledWith({ message: "sibling can send" });
      await act(async () => {
        finishResume();
        await restarting;
      });
      await act(async () => {
        expect(await result.current.submit.handleSubmit({ message: "send after restart" })).toBe(
          true,
        );
      });
      expect(onSend).toHaveBeenCalledWith({ message: "send after restart" });
    } finally {
      await act(async () => {
        finishStop();
        finishResume();
        await restarting;
      });
    }
  });
});
