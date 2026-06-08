import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { ExecutorProfile } from "@/lib/types/http";

const PROFILE_A: AgentProfileOption = {
  id: "profile-a",
  label: "Mock Agent \u2022 Fast",
  agent_name: "mock",
  agent_id: "agent-1",
  cli_passthrough: false,
};

const PROFILE_B: AgentProfileOption = {
  id: "profile-b",
  label: "Mock Agent \u2022 Slow",
  agent_name: "mock",
  agent_id: "agent-1",
  cli_passthrough: false,
};

let mockProfiles: AgentProfileOption[] = [PROFILE_A, PROFILE_B];
let mockExecutorProfile: ExecutorProfile | null = null;
let mockAuthLoaded = true;
let mockAuthSpecs: Record<string, unknown> = {};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      agentProfiles: { items: mockProfiles },
    }),
}));

vi.mock("@/hooks/domains/session/use-task-executor-profile", () => ({
  useTaskExecutorProfile: () => mockExecutorProfile,
}));

vi.mock("@/hooks/domains/settings/use-remote-auth-specs", () => ({
  useRemoteAuthSpecs: () => ({ specs: mockAuthSpecs, loaded: mockAuthLoaded }),
}));

vi.mock("@/lib/agent-executor-compat", () => ({
  isAgentConfiguredOnExecutor: (
    profile: AgentProfileOption,
    _executor: ExecutorProfile,
    _specs: Record<string, unknown>,
  ) => profile.id === "profile-a",
}));

import { useHandoffProfiles } from "./handoff-profile-menu-items";

describe("useHandoffProfiles", () => {
  afterEach(() => {
    mockProfiles = [PROFILE_A, PROFILE_B];
    mockExecutorProfile = null;
    mockAuthLoaded = true;
    mockAuthSpecs = {};
  });

  it("returns all agent profiles with display labels", () => {
    const { result } = renderHook(() => useHandoffProfiles("task-1"));
    expect(result.current).toHaveLength(2);
    expect(result.current[0]).toMatchObject({
      id: "profile-a",
      label: "Fast",
      agentName: "mock",
    });
    expect(result.current[1]).toMatchObject({
      id: "profile-b",
      label: "Slow",
    });
  });

  it("marks incompatible profiles disabled when executor profile is known", () => {
    mockExecutorProfile = {
      id: "exec-profile-1",
      name: "Default",
      executor_id: "executor-1",
      executor_type: "local_pc",
      prepare_script: "",
      cleanup_script: "",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };
    const { result } = renderHook(() => useHandoffProfiles("task-1"));
    expect(result.current.find((p) => p.id === "profile-a")?.disabled).toBe(false);
    expect(result.current.find((p) => p.id === "profile-b")?.disabled).toBe(true);
  });

  it("returns empty list when no profiles configured", () => {
    mockProfiles = [];
    const { result } = renderHook(() => useHandoffProfiles("task-1"));
    expect(result.current).toEqual([]);
  });
});
