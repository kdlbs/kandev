import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { TaskSession } from "@/lib/types/http";
import type { MessageAttachment } from "@/components/task/chat/chat-input-container";

const mockLaunchSession = vi.fn();
const mockSetChatDraftContent = vi.fn();
const mockToast = vi.fn();
const mockWsRequest = vi.fn();
const mockSetActiveSession = vi.fn();

let mockStoreState: {
  taskSessions: { items: Record<string, TaskSession> };
  setActiveSession: typeof mockSetActiveSession;
} = {
  taskSessions: { items: {} },
  setActiveSession: mockSetActiveSession,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof mockStoreState) => unknown) => selector(mockStoreState),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: (...args: unknown[]) => mockLaunchSession(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockWsRequest }),
}));

vi.mock("@/lib/local-storage", async (importOriginal) => {
  const actual = (await importOriginal()) as Record<string, unknown>;
  return {
    ...actual,
    setChatDraftContent: (...args: unknown[]) => mockSetChatDraftContent(...args),
  };
});

import { useImplementFresh } from "./use-implement-fresh";

const TASK_ID = "task-1";
const SESS_PLAN = "sess-plan";
const SESS_FRESH = "sess-fresh";

function makeSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: SESS_PLAN,
    task_id: TASK_ID,
    state: "WAITING_FOR_INPUT",
    agent_profile_id: "ap-1",
    executor_id: "ex-1",
    created_at: "",
    updated_at: "",
    ...overrides,
  } as TaskSession;
}

function makeChatRef(opts: { value?: string; attachments?: MessageAttachment[] } = {}) {
  const clear = vi.fn();
  const ref = {
    current: {
      focusInput: vi.fn(),
      getTextareaElement: () => null,
      getValue: () => opts.value ?? "",
      getSelectionStart: () => 0,
      insertText: vi.fn(),
      clear,
      getAttachments: () => opts.attachments ?? [],
    },
  };
  return { ref, clear };
}

function setup(session: TaskSession | undefined = makeSession()) {
  vi.clearAllMocks();
  mockLaunchSession.mockResolvedValue({
    success: true,
    task_id: TASK_ID,
    session_id: SESS_FRESH,
    state: "RUNNING",
  });
  mockWsRequest.mockResolvedValue({ success: true });
  mockStoreState = {
    taskSessions: { items: session ? { [session.id]: session } : {} },
    setActiveSession: mockSetActiveSession,
  };
}

describe("useImplementFresh", () => {
  beforeEach(() => setup());

  it("launches a fresh session inheriting agent + executor profile from the planning session", async () => {
    const { ref } = makeChatRef({ value: "double-check the migration" });
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(mockLaunchSession).toHaveBeenCalledTimes(1);
    expect(mockLaunchSession).toHaveBeenCalledWith(
      expect.objectContaining({
        task_id: TASK_ID,
        intent: "start",
        agent_profile_id: "ap-1",
        executor_id: "ex-1",
        plan_mode: false,
        prompt: expect.stringContaining("double-check the migration"),
      }),
    );
    expect(mockLaunchSession.mock.calls[0][0].prompt).toContain("<kandev-system>");
  });

  it("forwards attachments when present", async () => {
    const attachments: MessageAttachment[] = [
      { type: "image", data: "base64data", mime_type: "image/png" },
    ];
    const { ref } = makeChatRef({ value: "look at this", attachments });
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(mockLaunchSession.mock.calls[0][0].attachments).toEqual(attachments);
  });

  it("omits attachments key when none are present", async () => {
    const { ref } = makeChatRef({ value: "just text" });
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(mockLaunchSession.mock.calls[0][0]).not.toHaveProperty("attachments");
  });

  it("marks the fresh session as primary via WS after launch", async () => {
    const { ref } = makeChatRef({ value: "implement" });
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(mockWsRequest).toHaveBeenCalledWith(
      "session.set_primary",
      { session_id: SESS_FRESH },
      10000,
    );
  });

  it("focuses the fresh session as active in the UI after launch", async () => {
    const { ref } = makeChatRef({ value: "implement" });
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(mockSetActiveSession).toHaveBeenCalledWith(TASK_ID, SESS_FRESH);
  });

  it("clears composer + draft only on successful launch", async () => {
    const { ref, clear } = makeChatRef({ value: "ship" });
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(clear).toHaveBeenCalledTimes(1);
    expect(mockSetChatDraftContent).toHaveBeenCalledWith(SESS_PLAN, null);
  });

  it("preserves composer + draft when launch fails so user can retry", async () => {
    mockLaunchSession.mockRejectedValueOnce(new Error("network down"));
    const { ref, clear } = makeChatRef({ value: "important" });
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(clear).not.toHaveBeenCalled();
    expect(mockSetChatDraftContent).not.toHaveBeenCalled();
  });

  it("shows an error toast when launch fails", async () => {
    mockLaunchSession.mockRejectedValueOnce(new Error("timeout"));
    const { ref } = makeChatRef({ value: "implement" });
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(mockToast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  });

  it("continues on set_primary failure to avoid losing the launch", async () => {
    mockWsRequest.mockRejectedValueOnce(new Error("WS unavailable"));
    const { ref, clear } = makeChatRef({ value: "continue anyway" });
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    // Should still clear and set active even if set_primary fails
    expect(clear).toHaveBeenCalledTimes(1);
    expect(mockSetActiveSession).toHaveBeenCalledWith(TASK_ID, SESS_FRESH);
  });
});

describe("useImplementFresh guards", () => {
  beforeEach(() => setup());

  it("no-ops when session ID is missing", async () => {
    const { ref } = makeChatRef();
    const { result } = renderHook(() => useImplementFresh(null, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(mockLaunchSession).not.toHaveBeenCalled();
  });

  it("no-ops when planning session has no agent profile", async () => {
    setup(makeSession({ agent_profile_id: undefined }));
    const { ref } = makeChatRef();
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(mockLaunchSession).not.toHaveBeenCalled();
  });

  it("launches without executor_id when planning session is missing one (backend resolves)", async () => {
    setup(makeSession({ executor_id: undefined }));
    const { ref } = makeChatRef({ value: "go" });
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(mockLaunchSession).toHaveBeenCalledTimes(1);
    expect(mockLaunchSession.mock.calls[0][0]).not.toHaveProperty("executor_id");
  });

  it("does not clear composer when launchSession resolves without a session_id", async () => {
    mockLaunchSession.mockResolvedValueOnce({ success: true, task_id: TASK_ID, state: "RUNNING" });
    const { ref, clear } = makeChatRef({ value: "preserve me" });
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(clear).not.toHaveBeenCalled();
    expect(mockSetChatDraftContent).not.toHaveBeenCalled();
    expect(mockSetActiveSession).not.toHaveBeenCalled();
  });

  it("no-ops when planning session is not in the store", async () => {
    mockStoreState = { taskSessions: { items: {} }, setActiveSession: mockSetActiveSession }; // Empty store
    mockLaunchSession.mockClear();
    const { ref } = makeChatRef();
    const { result } = renderHook(() => useImplementFresh(SESS_PLAN, TASK_ID, ref));

    await act(async () => {
      await result.current();
    });

    expect(mockLaunchSession).not.toHaveBeenCalled();
  });
});
