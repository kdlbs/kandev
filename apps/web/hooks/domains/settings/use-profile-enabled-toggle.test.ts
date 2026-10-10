import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { Agent, AgentProfile } from "@/lib/types/http";
import { applyEnabledProfileUpdate, useProfileEnabledToggle } from "./use-profile-enabled-toggle";

const mocks = vi.hoisted(() => ({
  updateAgentProfileAction: vi.fn(),
  setState: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/app/actions/agents", () => ({
  updateAgentProfileAction: (...args: unknown[]) => mocks.updateAgentProfileAction(...args),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ setState: mocks.setState }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));

function profile(id: string, agentId: string, enabled: boolean): AgentProfile {
  return {
    id,
    agentId,
    name: id,
    agentDisplayName: agentId,
    model: "",
    allowIndexing: false,
    autoApprove: false,
    cliFlags: [],
    cliPassthrough: false,
    enabled,
    createdAt: "",
    updatedAt: "",
  } as unknown as AgentProfile;
}

function stateWith(...profiles: AgentProfile[]) {
  const agents = new Map<string, Agent>();
  for (const current of profiles) {
    const existing =
      agents.get(current.agentId) ??
      ({
        id: current.agentId,
        name: current.agentId,
        profiles: [],
      } as unknown as Agent);
    existing.profiles.push(current);
    agents.set(current.agentId, existing);
  }
  return {
    settingsAgents: { items: [...agents.values()] },
    agentProfiles: { items: [], version: 0, orderByAgent: {} },
  };
}

describe("applyEnabledProfileUpdate", () => {
  it("updates enabled state without copying saved Settings order into selectors", () => {
    const old = { ...profile("old", "a", true), createdAt: "2026-01-01T00:00:00Z" };
    const newer = { ...profile("new", "a", true), createdAt: "2026-02-01T00:00:00Z" };
    const next = applyEnabledProfileUpdate(stateWith(old, newer), old, { ...old, enabled: false });
    expect(next.agentProfiles.items.map((item) => item.id)).toEqual(["new", "old"]);
    expect(next.agentProfiles.items.find((item) => item.id === "old")?.enabled).toBe(false);
    expect(next.settingsAgents.items[0].profiles.map((item) => item.id)).toEqual(["old", "new"]);
  });

  it("merges concurrent responses from the latest state in either order", () => {
    const first = profile("p1", "a1", true);
    const second = profile("p2", "a2", true);
    const initial = stateWith(first, second);

    const firstThenSecond = applyEnabledProfileUpdate(
      applyEnabledProfileUpdate(initial, first, { ...first, enabled: false }),
      second,
      { ...second, enabled: false },
    );
    expect(
      firstThenSecond.settingsAgents.items.flatMap((agent) => agent.profiles).map((p) => p.enabled),
    ).toEqual([false, false]);

    const secondThenFirst = applyEnabledProfileUpdate(
      applyEnabledProfileUpdate(initial, second, { ...second, enabled: false }),
      first,
      { ...first, enabled: false },
    );
    expect(
      secondThenFirst.settingsAgents.items.flatMap((agent) => agent.profiles).map((p) => p.enabled),
    ).toEqual([false, false]);
  });
});

describe("useProfileEnabledToggle", () => {
  beforeEach(() => vi.clearAllMocks());

  it("does not toast a stale interlock error already handled by the coordinator", async () => {
    const error = new ApiError("stale document", 403, {
      error_code: "interim_settings_interlock_required",
    });
    error.handled = true;
    mocks.updateAgentProfileAction.mockRejectedValue(error);

    const { result } = renderHook(() => useProfileEnabledToggle());
    await act(async () => {
      await result.current(profile("p1", "a1", true), false);
    });

    expect(mocks.toast).not.toHaveBeenCalled();
  });
});
