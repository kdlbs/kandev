import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const startConfigChat = vi.fn();
const setTaskSession = vi.fn();
const openQuickChat = vi.fn();
const addQuickChatSession = vi.fn();
const closeQuickChatSession = vi.fn();
const renameQuickChatSession = vi.fn();
const setQuickChatInitialPrompt = vi.fn();
const deleteTask = vi.fn();
const WORKSPACE_ID = "workspace-1";
const CONFIG_PROFILE_ID = "profile-config";
const PASSTHROUGH_PROFILE_ID = "profile-passthrough";
const SESSION_ID = "session-config";
const TASK_ID = "task-config";
const PROMPT = "Show current workflows";

const appState = {
  openQuickChat,
  addQuickChatSession,
  closeQuickChatSession,
  renameQuickChatSession,
  setQuickChatInitialPrompt,
  setTaskSession,
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
}));

vi.mock("@/app/actions/workspaces", () => ({ updateWorkspaceAction: vi.fn() }));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: () => ({ agentProfiles: appState.agentProfiles.items }),
}));

vi.mock("@/hooks/domains/workspace/use-workspaces", () => ({
  useWorkspaces: () => ({ items: appState.workspaces.items }),
}));

import { useConfigChat } from "./use-config-chat";
import { getQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";

function renderConfigChatHook() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
  return renderHook(() => useConfigChat(WORKSPACE_ID), { wrapper });
}

beforeEach(() => {
  vi.clearAllMocks();
  appState.agentProfiles.items = [
    { id: CONFIG_PROFILE_ID, cli_passthrough: false },
    { id: PASSTHROUGH_PROFILE_ID, cli_passthrough: true },
  ];
  startConfigChat.mockResolvedValue({ task_id: TASK_ID, session_id: SESSION_ID });
});

vi.mock("@/lib/api/domains/kanban-api", () => ({
  deleteTask: (...args: unknown[]) => deleteTask(...args),
}));

describe("useConfigChat unified launch", () => {
  it("seeds and opens a typed Quick Chat session with one pending initial prompt", async () => {
    const { result } = renderConfigChatHook();

    await act(async () => {
      await result.current.startSession(CONFIG_PROFILE_ID, PROMPT);
    });

    expect(startConfigChat).toHaveBeenCalledWith(WORKSPACE_ID, {
      agent_profile_id: CONFIG_PROFILE_ID,
    });
    expect(setTaskSession).toHaveBeenCalledWith(
      expect.objectContaining({ id: SESSION_ID, task_id: TASK_ID }),
    );
    expect(closeQuickChatSession).toHaveBeenCalledWith(
      getQuickChatSetupSessionId(WORKSPACE_ID, "config"),
    );
    expect(openQuickChat).toHaveBeenCalledWith(
      SESSION_ID,
      WORKSPACE_ID,
      CONFIG_PROFILE_ID,
      "config",
    );
    expect(renameQuickChatSession).toHaveBeenCalledWith(SESSION_ID, PROMPT);
    expect(setQuickChatInitialPrompt).toHaveBeenCalledWith(SESSION_ID, PROMPT);
  });

  it("registers a floating configuration session without opening the large dialog", async () => {
    const { result } = renderConfigChatHook();

    await act(async () => {
      await result.current.startSession(CONFIG_PROFILE_ID, PROMPT, { openInQuickChat: false });
    });

    expect(addQuickChatSession).toHaveBeenCalledWith(
      SESSION_ID,
      WORKSPACE_ID,
      CONFIG_PROFILE_ID,
      "config",
    );
    expect(openQuickChat).not.toHaveBeenCalled();
    expect(setQuickChatInitialPrompt).toHaveBeenCalledWith(SESSION_ID, PROMPT);
  });

  it("starts a passthrough profile with its prompt instead of stranding it outside the terminal", async () => {
    const { result } = renderConfigChatHook();

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
    const { result } = renderConfigChatHook();

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
    const { result } = renderConfigChatHook();

    act(() => {
      void result.current.startSession(CONFIG_PROFILE_ID, PROMPT);
    });
    act(() => result.current.reset());
    await act(async () => {
      resolveStart({ task_id: TASK_ID, session_id: SESSION_ID });
      await Promise.resolve();
    });

    expect(deleteTask).toHaveBeenCalledWith(TASK_ID);
    expect(openQuickChat).not.toHaveBeenCalled();
    expect(setTaskSession).not.toHaveBeenCalled();
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
    const first = renderConfigChatHook();
    const second = renderConfigChatHook();
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
