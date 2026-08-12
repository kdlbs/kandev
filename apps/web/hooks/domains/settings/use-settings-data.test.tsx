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

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function useSettingsSnapshot() {
  useSettingsData();
  const agentsLoaded = useAppStore((state) => state.settingsData.agentsLoaded);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  return { agentsLoaded, agentProfiles };
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
      agentDisplayName: "Mock Agent",
      name: "Default",
      cliPassthrough: false,
    };
    mocks.listAgents.mockResolvedValueOnce({ agents: [], total: 0 }).mockResolvedValueOnce({
      agents: [{ id: "agent-1", name: "Mock Agent", profiles: [profile] }],
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
});
