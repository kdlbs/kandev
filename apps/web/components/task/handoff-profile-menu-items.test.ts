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
const LOCAL_EXECUTOR_PROFILE: ExecutorProfile = {
  id: "exec-profile-1",
  name: "Default",
  executor_id: "executor-1",
  executor_type: "local_pc",
  prepare_script: "",
  cleanup_script: "",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};
let mockExecutorProfile: ExecutorProfile | null = LOCAL_EXECUTOR_PROFILE;
let mockAuthLoaded = true;
let mockAuthSpecs: Record<string, unknown> = {};
const mockUseTaskExecutorProfile = vi.fn(
  (_taskId: string, _enabled?: boolean) => mockExecutorProfile,
);

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      features: { dynamicAgentRouting: true },
      agentProfiles: { items: mockProfiles },
    }),
}));

vi.mock("@/hooks/domains/session/use-task-executor-profile", () => ({
  useTaskExecutorProfile: (taskId: string, enabled?: boolean) =>
    mockUseTaskExecutorProfile(taskId, enabled),
}));

vi.mock("@/hooks/domains/settings/use-remote-auth-specs", () => ({
  useRemoteAuthSpecs: () => ({ specs: mockAuthSpecs, loaded: mockAuthLoaded }),
}));

vi.mock("@/lib/agent-executor-compat", () => ({
  shouldFilterHandoffByHostHealth: (executor: ExecutorProfile | null) =>
    Boolean(executor && ["local", "local_pc", "worktree"].includes(executor.executor_type ?? "")),
  isAgentConfiguredOnExecutor: (
    profile: AgentProfileOption,
    _executor: ExecutorProfile,
    _specs: Record<string, unknown>,
  ) => profile.id === "profile-a",
}));

import { useHandoffProfiles, useHasSelectableAgentProfiles } from "./handoff-profile-menu-items";

describe("useHandoffProfiles", () => {
  afterEach(() => {
    mockProfiles = [PROFILE_A, PROFILE_B];
    mockExecutorProfile = LOCAL_EXECUTOR_PROFILE;
    mockAuthLoaded = true;
    mockAuthSpecs = {};
    mockUseTaskExecutorProfile.mockClear();
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
    mockExecutorProfile = LOCAL_EXECUTOR_PROFILE;
    const { result } = renderHook(() => useHandoffProfiles("task-1"));
    expect(result.current.find((p) => p.id === "profile-a")?.disabled).toBe(false);
    expect(result.current.find((p) => p.id === "profile-b")?.disabled).toBe(true);
  });

  it("keeps unhealthy profiles visible for an executor that runs agents off-host", () => {
    mockExecutorProfile = { ...LOCAL_EXECUTOR_PROFILE, executor_type: "local_docker" };
    mockProfiles = [PROFILE_A, { ...PROFILE_B, capability_status: "not_installed" }];

    const { result } = renderHook(() => useHandoffProfiles("task-1"));

    expect(result.current.map((p) => p.id)).toEqual(["profile-a", "profile-b"]);
  });

  it("returns empty list when no profiles configured", () => {
    mockProfiles = [];
    const { result } = renderHook(() => useHandoffProfiles("task-1"));
    expect(result.current).toEqual([]);
  });

  it("omits disabled profiles from handoff choices", () => {
    mockProfiles = [PROFILE_A, { ...PROFILE_B, enabled: false }];
    const { result } = renderHook(() => useHandoffProfiles("task-1"));
    expect(result.current.map((p) => p.id)).toEqual(["profile-a"]);
  });

  it("passes the enabled flag to executor profile lookup", () => {
    renderHook(() => useHandoffProfiles("task-1", false));
    expect(mockUseTaskExecutorProfile).toHaveBeenCalledWith("task-1", false);
  });

  it.each(["not_installed", "auth_required", "failed", "not_configured"] as const)(
    "excludes profiles whose agent capability_status is %s",
    (capability_status) => {
      mockProfiles = [PROFILE_A, { ...PROFILE_B, capability_status }];
      const { result } = renderHook(() => useHandoffProfiles("task-1"));
      expect(result.current.map((p) => p.id)).toEqual(["profile-a"]);
    },
  );

  it.each(["ok", "probing", undefined] as const)(
    "keeps profiles whose agent capability_status is %s",
    (capability_status) => {
      mockProfiles = [PROFILE_A, { ...PROFILE_B, capability_status }];
      const { result } = renderHook(() => useHandoffProfiles("task-1"));
      expect(result.current.map((p) => p.id)).toEqual(["profile-a", "profile-b"]);
    },
  );
});

describe("useHasSelectableAgentProfiles", () => {
  afterEach(() => {
    mockProfiles = [PROFILE_A, PROFILE_B];
  });

  it("is true when a selectable profile exists, even if every agent is unhealthy", () => {
    mockProfiles = [
      { ...PROFILE_A, capability_status: "not_installed" },
      { ...PROFILE_B, capability_status: "failed" },
    ];
    const { result } = renderHook(() => useHasSelectableAgentProfiles());
    expect(result.current).toBe(true);
  });

  it("is false when there are no profiles at all", () => {
    mockProfiles = [];
    const { result } = renderHook(() => useHasSelectableAgentProfiles());
    expect(result.current).toBe(false);
  });

  it("is false when every profile is disabled (not selectable)", () => {
    mockProfiles = [
      { ...PROFILE_A, enabled: false },
      { ...PROFILE_B, enabled: false },
    ];
    const { result } = renderHook(() => useHasSelectableAgentProfiles());
    expect(result.current).toBe(false);
  });
});
