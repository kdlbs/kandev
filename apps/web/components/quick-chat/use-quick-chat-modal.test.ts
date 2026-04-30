import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

// Mocks must be declared before importing the hook so vi.mock hoists correctly.
const mockToast = vi.fn();
const mockStartQuickChat = vi.fn();
const mockDeleteTask = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  startQuickChat: (...args: unknown[]) => mockStartQuickChat(...args),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  deleteTask: (...args: unknown[]) => mockDeleteTask(...args),
}));

import { useAgentSelection } from "./use-quick-chat-modal";

const WORKSPACE_ID = "ws-1";

type MockStore = Parameters<typeof useAgentSelection>[1];

function makeStore(overrides: Partial<MockStore> = {}): MockStore {
  return {
    isOpen: true,
    sessions: [],
    activeSessionId: "",
    closeQuickChat: vi.fn(),
    closeQuickChatSession: vi.fn(),
    setActiveQuickChatSession: vi.fn(),
    renameQuickChatSession: vi.fn(),
    openQuickChat: vi.fn(),
    agentProfiles: [
      { id: "agent-a", label: "Agent A", agent_id: "a", agent_name: "Agent A" },
      { id: "agent-b", label: "Agent B", agent_id: "b", agent_name: "Agent B" },
    ] as MockStore["agentProfiles"],
    taskSessions: {},
    ...overrides,
  };
}

function flushPromises() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useAgentSelection — happy path", () => {
  it("opens the chat and clears pending state when the request resolves", async () => {
    const store = makeStore();
    mockStartQuickChat.mockResolvedValue({ task_id: "task-a", session_id: "sess-a" });
    const onSuccess = vi.fn();

    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    await act(async () => {
      await result.current.handleSelectAgent("agent-a", onSuccess);
    });

    expect(store.openQuickChat).toHaveBeenCalledWith("sess-a", WORKSPACE_ID, "agent-a");
    expect(store.renameQuickChatSession).toHaveBeenCalledWith("sess-a", expect.any(String));
    expect(onSuccess).toHaveBeenCalledTimes(1);
    expect(mockDeleteTask).not.toHaveBeenCalled();
    expect(result.current.pendingAgentId).toBeNull();
  });
});

describe("useAgentSelection — supersession", () => {
  it("rapid-pick: a newer pick deletes the older orphan task", async () => {
    const store = makeStore();
    let resolveFirst!: (v: { task_id: string; session_id: string }) => void;
    const firstPromise = new Promise<{ task_id: string; session_id: string }>((r) => {
      resolveFirst = r;
    });
    mockStartQuickChat
      .mockImplementationOnce(() => firstPromise)
      .mockResolvedValueOnce({ task_id: "task-b", session_id: "sess-b" });

    const onSuccessA = vi.fn();
    const onSuccessB = vi.fn();
    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    // Click A — request hangs.
    act(() => {
      void result.current.handleSelectAgent("agent-a", onSuccessA);
    });
    expect(result.current.pendingAgentId).toBe("agent-a");

    // Click B — supersedes A.
    await act(async () => {
      await result.current.handleSelectAgent("agent-b", onSuccessB);
    });
    expect(onSuccessB).toHaveBeenCalledTimes(1);
    expect(store.openQuickChat).toHaveBeenCalledWith("sess-b", WORKSPACE_ID, "agent-b");

    // Now A resolves — its onSuccess must NOT fire and the orphan task is deleted.
    await act(async () => {
      resolveFirst({ task_id: "task-a", session_id: "sess-a" });
      await flushPromises();
    });
    expect(onSuccessA).not.toHaveBeenCalled();
    expect(mockDeleteTask).toHaveBeenCalledWith("task-a");
    expect(store.openQuickChat).not.toHaveBeenCalledWith(
      "sess-a",
      expect.anything(),
      expect.anything(),
    );
  });

  it("reset() during in-flight request: resolved task is deleted, onSuccess not called", async () => {
    const store = makeStore();
    let resolveStart!: (v: { task_id: string; session_id: string }) => void;
    mockStartQuickChat.mockImplementationOnce(
      () =>
        new Promise<{ task_id: string; session_id: string }>((r) => {
          resolveStart = r;
        }),
    );

    const onSuccess = vi.fn();
    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    act(() => {
      void result.current.handleSelectAgent("agent-a", onSuccess);
    });
    expect(result.current.pendingAgentId).toBe("agent-a");

    // User does something that supersedes the in-flight pick (handleNewChat, tab switch, etc.).
    act(() => {
      result.current.reset();
    });
    expect(result.current.pendingAgentId).toBeNull();

    await act(async () => {
      resolveStart({ task_id: "task-a", session_id: "sess-a" });
      await flushPromises();
    });
    expect(onSuccess).not.toHaveBeenCalled();
    expect(store.openQuickChat).not.toHaveBeenCalled();
    expect(mockDeleteTask).toHaveBeenCalledWith("task-a");
  });
});

describe("useAgentSelection — error handling", () => {
  it("does not toast when a superseded request rejects (avoid noise from races)", async () => {
    const store = makeStore();
    let rejectStart!: (e: Error) => void;
    mockStartQuickChat.mockImplementationOnce(
      () =>
        new Promise<{ task_id: string; session_id: string }>((_resolve, reject) => {
          rejectStart = reject;
        }),
    );

    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    act(() => {
      void result.current.handleSelectAgent("agent-a", vi.fn());
    });
    act(() => {
      result.current.reset();
    });

    await act(async () => {
      rejectStart(new Error("network blew up"));
      await flushPromises();
    });

    expect(mockToast).not.toHaveBeenCalled();
  });

  it("toasts when the current (non-superseded) request rejects", async () => {
    const store = makeStore();
    mockStartQuickChat.mockRejectedValueOnce(new Error("server exploded"));
    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    await act(async () => {
      await result.current.handleSelectAgent("agent-a", vi.fn());
    });

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Failed to start quick chat",
        description: "server exploded",
        variant: "error",
      }),
    );
    expect(result.current.pendingAgentId).toBeNull();
  });
});
