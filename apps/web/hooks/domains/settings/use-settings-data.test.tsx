import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { StateProvider, useAppStore } from "@/components/state-provider";

const mocks = vi.hoisted(() => ({
  listAgents: vi.fn(),
  listAvailableAgents: vi.fn(),
  listExecutors: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  listAgents: mocks.listAgents,
  listAvailableAgents: mocks.listAvailableAgents,
  listExecutors: mocks.listExecutors,
}));

import { useSettingsData } from "./use-settings-data";

const MOCK_AGENT_NAME = "Mock Agent";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function useSettingsSnapshot() {
  useSettingsData();
  const agentsLoaded = useAppStore((state) => state.settingsData.agentsLoaded);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  return { agentsLoaded, agentProfiles, setAgentProfiles };
}

beforeEach(() => {
  vi.useFakeTimers();
  mocks.listAgents.mockReset();
  mocks.listAvailableAgents.mockReset().mockReturnValue(new Promise(() => {}));
  mocks.listExecutors.mockReset().mockResolvedValue({ executors: [] });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useSettingsData", () => {
  it("retries an empty agent list before declaring that no profiles exist", async () => {
    const profile = {
      id: "profile-1",
      agentDisplayName: MOCK_AGENT_NAME,
      name: "Default",
      cliPassthrough: false,
    };
    mocks.listAgents.mockResolvedValueOnce({ agents: [], total: 0 }).mockResolvedValueOnce({
      agents: [{ id: "agent-1", name: MOCK_AGENT_NAME, profiles: [profile] }],
      total: 1,
    });

    const { result } = renderHook(() => useSettingsSnapshot(), { wrapper });

    await act(async () => {
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(100);
      await Promise.resolve();
    });

    expect(mocks.listAgents).toHaveBeenCalledTimes(2);
    expect(result.current.agentsLoaded).toBe(true);
    expect(result.current.agentProfiles[0]?.id).toBe("profile-1");
  });

  it("uses the final attempt after exhausting every retry delay", async () => {
    mocks.listAgents.mockResolvedValue({ agents: [], total: 0 });

    const { result } = renderHook(() => useSettingsSnapshot(), { wrapper });

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(mocks.listAgents).toHaveBeenCalledTimes(5);
    expect(result.current.agentsLoaded).toBe(true);
    expect(result.current.agentProfiles).toEqual([]);
  });

  it("keeps a profile created by WebSocket while the initial list request is pending", async () => {
    let resolveAgents: (response: { agents: unknown[]; total: number }) => void;
    mocks.listAgents.mockReturnValue(
      new Promise((resolve) => {
        resolveAgents = resolve;
      }),
    );
    const { result } = renderHook(() => useSettingsSnapshot(), { wrapper });

    await act(async () => {
      await Promise.resolve();
    });
    act(() => {
      result.current.setAgentProfiles([
        {
          id: "profile-live",
          label: `${MOCK_AGENT_NAME} • Live profile`,
          agent_id: "agent-1",
          agent_name: MOCK_AGENT_NAME,
          cli_passthrough: false,
          inference_capable: true,
          updatedAt: "2026-08-26T18:00:01Z",
        },
      ]);
    });
    await act(async () => {
      resolveAgents!({
        agents: [
          {
            id: "agent-1",
            name: MOCK_AGENT_NAME,
            profiles: [
              {
                id: "profile-stale",
                agentDisplayName: MOCK_AGENT_NAME,
                name: "Stale profile",
                cliPassthrough: false,
              },
            ],
          },
        ],
        total: 1,
      });
      await Promise.resolve();
    });

    expect(result.current.agentProfiles.map((profile) => profile.id)).toContain("profile-live");
  });
});
