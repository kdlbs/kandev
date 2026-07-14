import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const startConfigChat = vi.fn();
const setTaskSession = vi.fn();
const openQuickChat = vi.fn();
const closeQuickChatSession = vi.fn();
const renameQuickChatSession = vi.fn();
const WORKSPACE_ID = "workspace-1";
const CONFIG_PROFILE_ID = "profile-config";
const PASSTHROUGH_PROFILE_ID = "profile-passthrough";
const SESSION_ID = "session-config";
const TASK_ID = "task-config";
const PROMPT = "Show current workflows";

const appState = {
  openQuickChat,
  closeQuickChatSession,
  renameQuickChatSession,
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

import { useConfigChat } from "./use-config-chat";

beforeEach(() => {
  vi.clearAllMocks();
  startConfigChat.mockResolvedValue({ task_id: TASK_ID, session_id: SESSION_ID });
});

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
    expect(closeQuickChatSession).toHaveBeenCalledWith("");
    expect(openQuickChat).toHaveBeenCalledWith(
      SESSION_ID,
      WORKSPACE_ID,
      CONFIG_PROFILE_ID,
      "config",
    );
    expect(renameQuickChatSession).toHaveBeenCalledWith(SESSION_ID, PROMPT);
    expect(result.current.pendingPrompt).toEqual({
      sessionId: SESSION_ID,
      prompt: PROMPT,
    });
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
    expect(result.current.pendingPrompt).toBeNull();
  });
});
