import { describe, expect, it, vi } from "vitest";
import type { Agent, AgentProfile } from "@/lib/types/http";
import { applyEnabledProfileUpdate } from "./use-profile-enabled-toggle";

vi.mock("@/app/actions/agents", () => ({ updateAgentProfileAction: vi.fn() }));
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: vi.fn() }));
vi.mock("@/components/toast-provider", () => ({ useToast: vi.fn() }));

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
    agentProfiles: { items: [], version: 0 },
  };
}

describe("applyEnabledProfileUpdate", () => {
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
