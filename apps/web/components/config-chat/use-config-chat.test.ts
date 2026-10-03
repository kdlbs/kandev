import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";

const startConfigChat = vi.fn();
const restartConfigChat = vi.fn();
const listQuickChatSessions = vi.fn();
const getTaskDeletePreflight = vi.fn();
const replaceConfigChatSession = vi.fn();
const removeQuickChatSession = vi.fn();
const setConfigChatRestart = vi.fn();
const setTaskSession = vi.fn();
const removeTaskSession = vi.fn();
const openQuickChat = vi.fn();
const addQuickChatSession = vi.fn();
const closeQuickChatSession = vi.fn();
const renameQuickChatSession = vi.fn();
const setQuickChatInitialPrompt = vi.fn();
const applyAgentProfileRecentUse = vi.fn();
const deleteTask = vi.fn();
const recordRecentUseMock = vi.fn();
const WORKSPACE_ID = "workspace-1";
const CONFIG_PROFILE_ID = "profile-config";
const PASSTHROUGH_PROFILE_ID = "profile-passthrough";
const SESSION_ID = "session-config";
const TASK_ID = "task-config";
const PROMPT = "Show current workflows";
const REPLACEMENT_TASK_ID = "replacement-task";
const REPLACEMENT_SESSION_ID = "replacement-session";
const existing = {
  sessionId: SESSION_ID,
  taskId: TASK_ID,
  workspaceId: WORKSPACE_ID,
  kind: "config" as const,
  agentProfileId: CONFIG_PROFILE_ID,
};

const appState = {
  quickChat: {
    sessions: [
      {
        sessionId: SESSION_ID,
        taskId: TASK_ID,
        workspaceId: WORKSPACE_ID,
        kind: "config",
        agentProfileId: CONFIG_PROFILE_ID,
      },
    ],
    configChatRestarts: {} as Record<
      string,
      { sessionId: string; status: string; source: string; error?: string }
    >,
  },
  setConfigChatRestart,
  replaceConfigChatSession,
  removeQuickChatSession,
  openQuickChat,
  addQuickChatSession,
  closeQuickChatSession,
  renameQuickChatSession,
  setQuickChatInitialPrompt,
  applyAgentProfileRecentUse,
  setTaskSession,
  removeTaskSession,
  environmentIdBySessionId: {},
  taskSessions: {
    items: {} as Record<string, { cancellation_pending?: boolean }>,
  },
  agentProfiles: {
    items: [
      { id: CONFIG_PROFILE_ID, cli_passthrough: false },
      { id: PASSTHROUGH_PROFILE_ID, cli_passthrough: true },
    ],
  },
  workspaces: {
    items: [
      {
        id: WORKSPACE_ID,
        default_config_agent_profile_id: CONFIG_PROFILE_ID,
        default_agent_profile_id: "profile-default",
      },
    ],
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof appState) => unknown) => selector(appState),
  useAppStoreApi: () => ({ getState: () => appState }),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  startConfigChat: (...args: unknown[]) => startConfigChat(...args),
  restartConfigChat: (...args: unknown[]) => restartConfigChat(...args),
  listQuickChatSessions: (...args: unknown[]) => listQuickChatSessions(...args),
}));

vi.mock("@/lib/agent-profile-recent-use", () => ({
  recordAgentProfileRecentUseBestEffort: (...args: unknown[]) => recordRecentUseMock(...args),
}));

vi.mock("@/app/actions/workspaces", () => ({ updateWorkspaceAction: vi.fn() }));

import { useConfigChat } from "./use-config-chat";
import { getQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";

beforeEach(() => {
  vi.clearAllMocks();
  recordRecentUseMock.mockReset();
  appState.agentProfiles.items = [
    { id: CONFIG_PROFILE_ID, cli_passthrough: false },
    { id: PASSTHROUGH_PROFILE_ID, cli_passthrough: true },
  ];
  appState.taskSessions.items = {};
  appState.quickChat.configChatRestarts = {};
  setConfigChatRestart.mockImplementation(
    (
      workspaceId: string,
      value: { sessionId: string; status: string; source: string; error?: string } | null,
    ) => {
      if (value) appState.quickChat.configChatRestarts[workspaceId] = value;
      else delete appState.quickChat.configChatRestarts[workspaceId];
    },
  );
  getTaskDeletePreflight.mockResolvedValue({
    confirmation_id: "confirmation",
    requires_discard_consent: false,
  });
  restartConfigChat.mockResolvedValue({
    task_id: REPLACEMENT_TASK_ID,
    session_id: REPLACEMENT_SESSION_ID,
    agent_profile_id: CONFIG_PROFILE_ID,
  });
  listQuickChatSessions.mockResolvedValue({
    sessions: [],
    task_sessions: [],
    config_chat_restart_pending: false,
  });
  setTaskSession.mockImplementation((session: { id: string; cancellation_pending?: boolean }) => {
    appState.taskSessions.items[session.id] = session;
  });
  startConfigChat.mockResolvedValue({ task_id: TASK_ID, session_id: SESSION_ID });
});

vi.mock("@/lib/api/domains/kanban-api", () => ({
  getTaskDeletePreflight: (...args: unknown[]) => getTaskDeletePreflight(...args),
  deleteTask: (...args: unknown[]) => deleteTask(...args),
  deleteTaskAfterUserAction: (...args: unknown[]) => deleteTask(...args),
}));

describe("useConfigChat unified launch", () => {
  it("seeds and opens a typed Quick Chat session with one pending initial prompt", async () => {
    const { result } = renderHook(() => useConfigChat(WORKSPACE_ID));

    await act(async () => {
      await result.current.startSession(CONFIG_PROFILE_ID, PROMPT);
    });

    expect(startConfigChat).toHaveBeenCalledWith(WORKSPACE_ID, {
      agent_profile_id: CONFIG_PROFILE_ID,
    });
    expect(setTaskSession).toHaveBeenCalledWith(
      expect.objectContaining({ id: SESSION_ID, task_id: TASK_ID }),
    );
    expect(appState.taskSessions.items[SESSION_ID]).toEqual(
      expect.objectContaining({ cancellation_pending: false }),
    );
    expect(closeQuickChatSession).toHaveBeenCalledWith(
      getQuickChatSetupSessionId(WORKSPACE_ID, "config"),
    );
    expect(openQuickChat).toHaveBeenCalledWith(
      SESSION_ID,
      WORKSPACE_ID,
      CONFIG_PROFILE_ID,
      "config",
      TASK_ID,
    );
    expect(renameQuickChatSession).toHaveBeenCalledWith(SESSION_ID, PROMPT);
    expect(setQuickChatInitialPrompt).toHaveBeenCalledWith(SESSION_ID, PROMPT);
    expect(recordRecentUseMock).toHaveBeenCalledWith(
      "config_chat",
      CONFIG_PROFILE_ID,
      expect.any(Function),
    );
  });

  it("registers a floating configuration session without opening the large dialog", async () => {
    const { result } = renderHook(() => useConfigChat(WORKSPACE_ID));

    await act(async () => {
      await result.current.startSession(CONFIG_PROFILE_ID, PROMPT, { openInQuickChat: false });
    });

    expect(addQuickChatSession).toHaveBeenCalledWith(
      SESSION_ID,
      WORKSPACE_ID,
      CONFIG_PROFILE_ID,
      "config",
      TASK_ID,
    );
    expect(openQuickChat).not.toHaveBeenCalled();
    expect(setQuickChatInitialPrompt).toHaveBeenCalledWith(SESSION_ID, PROMPT);
  });

  it("starts a passthrough profile with its prompt instead of stranding it outside the terminal", async () => {
    const { result } = renderHook(() => useConfigChat(WORKSPACE_ID));

    await act(async () => {
      await result.current.startSession(PASSTHROUGH_PROFILE_ID, PROMPT);
    });

    expect(startConfigChat).toHaveBeenCalledWith(WORKSPACE_ID, {
      agent_profile_id: PASSTHROUGH_PROFILE_ID,
      prompt: PROMPT,
    });
    expect(setQuickChatInitialPrompt).not.toHaveBeenCalled();
  });

  it("waits for the selected profile before deciding how to deliver the prompt", async () => {
    appState.agentProfiles.items = [];
    const { result } = renderHook(() => useConfigChat(WORKSPACE_ID));

    await act(async () => {
      await result.current.startSession(PASSTHROUGH_PROFILE_ID, PROMPT);
    });

    expect(startConfigChat).not.toHaveBeenCalled();
    expect(result.current.error).toMatch(/profile/i);
  });

  it("deletes a task that resolves after the config start is superseded", async () => {
    let resolveStart!: (value: { task_id: string; session_id: string }) => void;
    startConfigChat.mockImplementationOnce(
      () =>
        new Promise<{ task_id: string; session_id: string }>((resolve) => {
          resolveStart = resolve;
        }),
    );
    deleteTask.mockResolvedValue(undefined);
    const { result } = renderHook(() => useConfigChat(WORKSPACE_ID));

    act(() => {
      void result.current.startSession(CONFIG_PROFILE_ID, PROMPT);
    });
    act(() => result.current.reset());
    await act(async () => {
      resolveStart({ task_id: TASK_ID, session_id: SESSION_ID });
      await Promise.resolve();
    });

    expect(deleteTask).toHaveBeenCalledWith(TASK_ID);
    expect(recordRecentUseMock).not.toHaveBeenCalled();
    expect(openQuickChat).not.toHaveBeenCalled();
    expect(setTaskSession).not.toHaveBeenCalled();
  });
});

describe("useConfigChat restart", () => {
  it("replaces only the confirmed conversation without replaying its prompt or opening the dialog", async () => {
    const { result } = renderHook(() => useConfigChat(WORKSPACE_ID));
    await act(async () => {
      await result.current.restartSession(existing);
    });
    expect(getTaskDeletePreflight).toHaveBeenCalledWith([TASK_ID], false, false);
    expect(restartConfigChat).toHaveBeenCalledWith(
      WORKSPACE_ID,
      { task_id: TASK_ID, session_id: SESSION_ID },
      "confirmation",
    );
    expect(replaceConfigChatSession).toHaveBeenCalledWith(
      WORKSPACE_ID,
      SESSION_ID,
      expect.objectContaining({
        sessionId: REPLACEMENT_SESSION_ID,
        taskId: REPLACEMENT_TASK_ID,
        kind: "config",
      }),
    );
    expect(openQuickChat).not.toHaveBeenCalled();
    expect(setQuickChatInitialPrompt).not.toHaveBeenCalled();
    expect(startConfigChat).not.toHaveBeenCalled();
    expect(removeTaskSession).toHaveBeenCalledExactlyOnceWith(TASK_ID, SESSION_ID);
  });

  it("holds shared admission across restart clicks and configuration starts", async () => {
    let resolve!: (response: { task_id: string; session_id: string }) => void;
    restartConfigChat.mockImplementationOnce(
      () =>
        new Promise((r) => {
          resolve = r;
        }),
    );
    const first = renderHook(() => useConfigChat(WORKSPACE_ID));
    const second = renderHook(() => useConfigChat(WORKSPACE_ID));
    let pending!: Promise<void>;
    await act(async () => {
      pending = first.result.current.restartSession(existing);
      await Promise.resolve();
    });
    await act(async () => {
      await second.result.current.restartSession(existing);
      await second.result.current.startSession(CONFIG_PROFILE_ID, PROMPT);
    });
    expect(restartConfigChat).toHaveBeenCalledTimes(1);
    expect(startConfigChat).not.toHaveBeenCalled();
    expect(appState.quickChat.configChatRestarts[WORKSPACE_ID]).toMatchObject({
      sessionId: SESSION_ID,
      status: "restarting",
    });
    await act(async () => {
      resolve({ task_id: REPLACEMENT_TASK_ID, session_id: REPLACEMENT_SESSION_ID });
      await pending;
    });
  });

  it("keeps accepted replacement reachable when the panel closes or workspace changes", async () => {
    let resolve!: (response: { task_id: string; session_id: string }) => void;
    restartConfigChat.mockImplementationOnce(
      () =>
        new Promise((r) => {
          resolve = r;
        }),
    );
    const hook = renderHook(({ workspaceId }) => useConfigChat(workspaceId), {
      initialProps: { workspaceId: WORKSPACE_ID },
    });
    let pending!: Promise<void>;
    await act(async () => {
      pending = hook.result.current.restartSession(existing);
      await Promise.resolve();
    });
    act(() => hook.result.current.reset());
    hook.rerender({ workspaceId: "different-workspace" });
    await act(async () => {
      resolve({ task_id: REPLACEMENT_TASK_ID, session_id: REPLACEMENT_SESSION_ID });
      await pending;
    });
    expect(replaceConfigChatSession).toHaveBeenCalledWith(
      WORKSPACE_ID,
      SESSION_ID,
      expect.objectContaining({ sessionId: REPLACEMENT_SESSION_ID }),
    );
    expect(deleteTask).not.toHaveBeenCalled();
    expect(openQuickChat).not.toHaveBeenCalled();
  });
});

describe("useConfigChat restart reconciliation", () => {
  it("reconciles a lost response by adopting the server's unique replacement", async () => {
    restartConfigChat.mockRejectedValueOnce(new TypeError("network disconnected"));
    listQuickChatSessions.mockResolvedValueOnce({
      sessions: [
        {
          session_id: REPLACEMENT_SESSION_ID,
          task_id: REPLACEMENT_TASK_ID,
          workspace_id: WORKSPACE_ID,
          kind: "config",
          agent_profile_id: CONFIG_PROFILE_ID,
        },
      ],
      task_sessions: [],
      config_chat_restart_pending: false,
    });
    const { result } = renderHook(() => useConfigChat(WORKSPACE_ID));
    await act(async () => {
      await result.current.restartSession(existing);
    });
    expect(restartConfigChat).toHaveBeenCalledTimes(1);
    expect(replaceConfigChatSession).toHaveBeenCalled();
    expect(removeTaskSession).toHaveBeenCalledExactlyOnceWith(TASK_ID, SESSION_ID);
    expect(appState.quickChat.configChatRestarts[WORKSPACE_ID]).toBeUndefined();
  });

  it("blocks creation after an uncertain restart until a refresh succeeds", async () => {
    restartConfigChat.mockRejectedValueOnce(new TypeError("network disconnected"));
    listQuickChatSessions.mockRejectedValueOnce(new TypeError("offline"));
    const { result } = renderHook(() => useConfigChat(WORKSPACE_ID));
    await act(async () => {
      await result.current.restartSession(existing);
    });
    expect(appState.quickChat.configChatRestarts[WORKSPACE_ID]).toMatchObject({
      status: "uncertain",
    });
    await act(async () => {
      await result.current.startSession(CONFIG_PROFILE_ID, PROMPT);
    });
    expect(startConfigChat).not.toHaveBeenCalled();
    listQuickChatSessions.mockResolvedValueOnce({
      sessions: [],
      task_sessions: [],
      config_chat_restart_pending: false,
    });
    await act(async () => {
      await result.current.refreshRestart();
    });
    expect(removeQuickChatSession).toHaveBeenCalledWith(SESSION_ID);
    expect(appState.quickChat.configChatRestarts[WORKSPACE_ID]).toBeUndefined();
  });
});

describe("useConfigChat restart failures", () => {
  it.each([
    ["validate", false, false],
    ["stop", false, false],
    ["delete", false, false],
    ["create", true, false],
    ["start", true, true],
  ])("preserves reachable state after a %s failure", async (stage, oldDeleted, hasReplacement) => {
    restartConfigChat.mockRejectedValueOnce(
      new ApiError("restart failed", 500, {
        code: `config_chat_restart_${stage}_failed`,
        stage,
        old_deleted: oldDeleted,
        ...(hasReplacement
          ? { replacement: { task_id: REPLACEMENT_TASK_ID, session_id: REPLACEMENT_SESSION_ID } }
          : {}),
      }),
    );
    const { result } = renderHook(() => useConfigChat(WORKSPACE_ID));
    await act(async () => {
      await result.current.restartSession(existing);
    });
    expect(result.current.error).toBeTruthy();
    expect(listQuickChatSessions).not.toHaveBeenCalled();
    expect(appState.quickChat.configChatRestarts[WORKSPACE_ID]).toBeUndefined();
    if (hasReplacement) expect(replaceConfigChatSession).toHaveBeenCalled();
    else if (oldDeleted) expect(removeQuickChatSession).toHaveBeenCalledWith(SESSION_ID);
    else {
      expect(removeQuickChatSession).not.toHaveBeenCalled();
      expect(replaceConfigChatSession).not.toHaveBeenCalled();
    }
    expect(startConfigChat).not.toHaveBeenCalled();
  });

  it("leaves the conversation untouched when deletion preflight fails", async () => {
    getTaskDeletePreflight.mockRejectedValueOnce(new Error("preflight failed"));
    const { result } = renderHook(() => useConfigChat(WORKSPACE_ID));
    await act(async () => {
      await result.current.restartSession(existing);
    });
    expect(result.current.error).toBeTruthy();
    expect(restartConfigChat).not.toHaveBeenCalled();
    expect(listQuickChatSessions).not.toHaveBeenCalled();
    expect(removeQuickChatSession).not.toHaveBeenCalled();
  });
});

describe("useConfigChat launch serialization", () => {
  it("serializes config starts across hook instances in the same workspace", async () => {
    let resolveStart!: (value: { task_id: string; session_id: string }) => void;
    startConfigChat.mockImplementationOnce(
      () =>
        new Promise<{ task_id: string; session_id: string }>((resolve) => {
          resolveStart = resolve;
        }),
    );
    const first = renderHook(() => useConfigChat(WORKSPACE_ID));
    const second = renderHook(() => useConfigChat(WORKSPACE_ID));
    let firstStart!: Promise<string | undefined>;

    act(() => {
      firstStart = first.result.current.startSession(CONFIG_PROFILE_ID, PROMPT);
    });
    await act(async () => {
      await second.result.current.startSession(CONFIG_PROFILE_ID, PROMPT);
    });

    expect(startConfigChat).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveStart({ task_id: TASK_ID, session_id: SESSION_ID });
      await firstStart;
      await second.result.current.startSession(CONFIG_PROFILE_ID, PROMPT);
    });

    expect(startConfigChat).toHaveBeenCalledTimes(2);
  });
});
