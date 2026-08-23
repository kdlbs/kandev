import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useHealthyAgentProfiles } from "./use-healthy-agent-profiles";

const state = {
  features: { dynamicAgentRouting: false },
  agentProfiles: {
    items: [
      { id: "concrete", kind: "concrete", capability_status: "ok" },
      { id: "dynamic", kind: "dynamic", capability_status: "ok" },
    ],
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

afterEach(() => {
  cleanup();
  state.features.dynamicAgentRouting = false;
});

describe("useHealthyAgentProfiles", () => {
  it("hides dynamic profiles from new workflow selections while the flag is off", () => {
    const { result } = renderHook(() => useHealthyAgentProfiles());

    expect(result.current.map((profile) => profile.id)).toEqual(["concrete"]);
  });

  it("includes dynamic profiles when routing is enabled", () => {
    state.features.dynamicAgentRouting = true;

    const { result } = renderHook(() => useHealthyAgentProfiles());

    expect(result.current.map((profile) => profile.id)).toEqual(["concrete", "dynamic"]);
  });
});
