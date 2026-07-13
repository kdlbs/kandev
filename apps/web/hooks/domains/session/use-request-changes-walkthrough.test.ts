import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockRequest = vi.fn();
const mockQueueMessage = vi.fn();
const mockAddMessage = vi.fn();
const mockToast = vi.fn();
const mockListPrompts = vi.fn();
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
  queueMessage: (...args: unknown[]) => mockQueueMessage(...args),
}));

vi.mock("@/lib/api", () => ({
  listPrompts: (...args: unknown[]) => mockListPrompts(...args),
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

function renderRequestHook(ready = true) {
  return renderHook(() =>
    useRequestChangesWalkthrough({
      taskId: "task-1",
      sessionId: "session-1",
      ready,
    }),
  );
}

describe("useRequestChangesWalkthrough", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockStoreState = storeState("WAITING_FOR_INPUT");
    mockListPrompts.mockResolvedValue({
      prompts: [
        {
          id: "builtin-changes-walkthrough",
          name: "changes-walkthrough",
          content: "CUSTOM_WALKTHROUGH_PROMPT\nshow_walkthrough_kandev",
          builtin: true,
        },
      ],
    });
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
        content: expect.stringMatching(/^@changes-walkthrough\n\n<kandev-system>/),
      }),
      10000,
    );
    const sentContent = mockRequest.mock.calls[0]?.[1]?.content as string;
    expect(sentContent).toContain("<kandev-system>");
    expect(sentContent).toContain("CUSTOM_WALKTHROUGH_PROMPT");
    expect(sentContent).not.toContain("Diff context:");
    expect(sentContent).not.toContain("Base branch:");
    expect(sentContent).not.toContain("Base commit:");
    expect(mockListPrompts).toHaveBeenCalledWith({ cache: "no-store" });
    expect(mockAddMessage).toHaveBeenCalledWith(expect.objectContaining({ id: "msg-1" }));
    expect(mockQueueMessage).not.toHaveBeenCalled();
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

    expect(mockQueueMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        session_id: "session-1",
        task_id: "task-1",
        plan_mode: true,
        content: expect.stringMatching(/^@changes-walkthrough\n\n<kandev-system>/),
      }),
    );
    const queuedContent = mockQueueMessage.mock.calls[0]?.[0]?.content as string;
    expect(queuedContent).toContain("<kandev-system>");
    expect(queuedContent).toContain("CUSTOM_WALKTHROUGH_PROMPT");
    expect(mockRequest).not.toHaveBeenCalled();
    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Walkthrough request queued" }),
    );
  });

  it("does not send a low-context prompt before diff context is ready", async () => {
    const { result } = renderRequestHook(false);

    await act(async () => {
      await result.current();
    });

    expect(mockRequest).not.toHaveBeenCalled();
    expect(mockQueueMessage).not.toHaveBeenCalled();
    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Changes are still loading", variant: "error" }),
    );
    expect(mockListPrompts).not.toHaveBeenCalled();
  });
});
