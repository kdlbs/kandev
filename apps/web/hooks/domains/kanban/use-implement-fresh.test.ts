import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { TaskSession } from "@/lib/types/http";
import type { MessageAttachment } from "@/components/task/chat/chat-input-container";

const mockLaunchSession = vi.fn();
const mockSetChatDraftContent = vi.fn();

let mockStoreState: { taskSessions: { items: Record<string, TaskSession> } } = {
  taskSessions: { items: {} },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof mockStoreState) => unknown) => selector(mockStoreState),
}));

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: (...args: unknown[]) => mockLaunchSession(...args),
}));

vi.mock("@/lib/local-storage", async (importOriginal) => {
  const actual = (await importOriginal()) as Record<string, unknown>;
  return {
    ...actual,
    setChatDraftContent: (...args: unknown[]) => mockSetChatDraftContent(...args),
  };
});

import { useImplementFresh } from "./use-implement-fresh";

function makeSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: "sess-plan",
    task_id: "task-1",
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
  mockLaunchSession.mockResolvedValue({ success: true, task_id: "task-1", state: "RUNNING" });
  mockStoreState = { taskSessions: { items: session ? { [session.id]: session } : {} } };
}

describe("useImplementFresh", () => {
  beforeEach(() => setup());

  it("launches a fresh session inheriting agent + executor profile from the planning session", async () => {
    const { ref } = makeChatRef({ value: "double-check the migration" });
    const { result } = renderHook(() => useImplementFresh("sess-plan", "task-1", ref));

    await act(async () => {
      await result.current();
    });

    expect(mockLaunchSession).toHaveBeenCalledTimes(1);
    expect(mockLaunchSession).toHaveBeenCalledWith(
      expect.objectContaining({
        task_id: "task-1",
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
    const { result } = renderHook(() => useImplementFresh("sess-plan", "task-1", ref));

    await act(async () => {
      await result.current();
    });

    expect(mockLaunchSession.mock.calls[0][0].attachments).toEqual(attachments);
  });

  it("omits attachments key when none are present", async () => {
    const { ref } = makeChatRef({ value: "just text" });
    const { result } = renderHook(() => useImplementFresh("sess-plan", "task-1", ref));

    await act(async () => {
      await result.current();
    });

    expect(mockLaunchSession.mock.calls[0][0]).not.toHaveProperty("attachments");
  });

  it("clears composer + draft only on successful launch", async () => {
    const { ref, clear } = makeChatRef({ value: "ship" });
    const { result } = renderHook(() => useImplementFresh("sess-plan", "task-1", ref));

    await act(async () => {
      await result.current();
    });

    expect(clear).toHaveBeenCalledTimes(1);
    expect(mockSetChatDraftContent).toHaveBeenCalledWith("sess-plan", null);
  });

  it("preserves composer + draft when launch fails so user can retry", async () => {
    mockLaunchSession.mockRejectedValueOnce(new Error("network down"));
    const { ref, clear } = makeChatRef({ value: "important" });
    const { result } = renderHook(() => useImplementFresh("sess-plan", "task-1", ref));

    await act(async () => {
      await result.current();
    });

    expect(clear).not.toHaveBeenCalled();
    expect(mockSetChatDraftContent).not.toHaveBeenCalled();
  });

  it("no-ops when session ID is missing", async () => {
    const { ref } = makeChatRef();
    const { result } = renderHook(() => useImplementFresh(null, "task-1", ref));

    await act(async () => {
      await result.current();
    });

    expect(mockLaunchSession).not.toHaveBeenCalled();
  });

  it("no-ops when planning session has no agent profile", async () => {
    setup(makeSession({ agent_profile_id: undefined }));
    const { ref } = makeChatRef();
    const { result } = renderHook(() => useImplementFresh("sess-plan", "task-1", ref));

    await act(async () => {
      await result.current();
    });

    expect(mockLaunchSession).not.toHaveBeenCalled();
  });

  it("no-ops when planning session is not in the store", async () => {
    mockStoreState = { taskSessions: { items: {} } }; // Empty store - no session with ID "sess-plan"
    mockLaunchSession.mockClear();
    const { ref } = makeChatRef();
    const { result } = renderHook(() => useImplementFresh("sess-plan", "task-1", ref));

    await act(async () => {
      await result.current();
    });

    // When session not found, planningSession?.agent_profile_id is undefined, preventing the call
    expect(mockLaunchSession).not.toHaveBeenCalled();
  });
});
