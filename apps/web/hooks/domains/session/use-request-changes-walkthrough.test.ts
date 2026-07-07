import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockRequest = vi.fn();
const mockAppendToQueue = vi.fn();
const mockAddMessage = vi.fn();
const mockToast = vi.fn();
const mockGetWebSocketClient = vi.fn(() => ({ request: mockRequest }));
type MockStoreState = ReturnType<typeof storeState>;
let mockStoreState: MockStoreState;

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => mockStoreState }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/api/domains/queue-api", () => ({
  appendToQueue: (...args: unknown[]) => mockAppendToQueue(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockGetWebSocketClient(),
}));

import { useRequestChangesWalkthrough } from "./use-request-changes-walkthrough";

function storeState(sessionState: string, planMode = false) {
  return {
    taskSessions: { items: { "session-1": { state: sessionState } } },
    chatInput: { planModeBySessionId: { "session-1": planMode } },
    addMessage: mockAddMessage,
  };
}

function renderRequestHook() {
  return renderHook(() =>
    useRequestChangesWalkthrough({
      taskId: "task-1",
      sessionId: "session-1",
      files: [{ path: "src/app.ts", source: "uncommitted" }],
    }),
  );
}

describe("useRequestChangesWalkthrough", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockStoreState = storeState("WAITING_FOR_INPUT");
  });

  it("sends a walkthrough request directly when the agent is waiting", async () => {
    mockRequest.mockResolvedValueOnce({
      id: "msg-1",
      session_id: "session-1",
      task_id: "task-1",
      type: "message",
      author_type: "user",
      content: "prompt",
      created_at: "2026-07-07T00:00:00Z",
    });
    const { result } = renderRequestHook();

    await act(async () => {
      await result.current();
    });

    expect(mockRequest).toHaveBeenCalledWith(
      "message.add",
      expect.objectContaining({
        task_id: "task-1",
        session_id: "session-1",
        content: expect.stringContaining("show_walkthrough_kandev"),
      }),
      10000,
    );
    expect(mockAddMessage).toHaveBeenCalledWith(expect.objectContaining({ id: "msg-1" }));
    expect(mockAppendToQueue).not.toHaveBeenCalled();
    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Walkthrough request sent" }),
    );
  });

  it("queues a walkthrough request when the agent is running", async () => {
    mockStoreState = storeState("RUNNING", true);
    const { result } = renderRequestHook();

    await act(async () => {
      await result.current();
    });

    expect(mockAppendToQueue).toHaveBeenCalledWith(
      expect.objectContaining({
        session_id: "session-1",
        task_id: "task-1",
        plan_mode: true,
        content: expect.stringContaining("show_walkthrough_kandev"),
      }),
    );
    expect(mockRequest).not.toHaveBeenCalled();
    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Walkthrough request queued" }),
    );
  });
});
