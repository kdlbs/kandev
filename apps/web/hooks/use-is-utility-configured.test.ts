import { describe, expect, it, vi } from "vitest";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";

type MockState = { userSettings: UserSettingsState };

let mockState: MockState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

import { useIsUtilityConfigured } from "./use-is-utility-configured";

function baseSettings(): UserSettingsState {
  return {
    defaultUtilityAgentId: null,
    defaultUtilityAgentProfileId: null,
  } as UserSettingsState;
}

describe("useIsUtilityConfigured", () => {
  it("is configured when only the default utility agent profile id is set", () => {
    mockState = {
      userSettings: { ...baseSettings(), defaultUtilityAgentProfileId: "profile-1" },
    };

    expect(useIsUtilityConfigured()).toBe(true);
  });

  it("is configured when only the legacy default utility agent id is set", () => {
    mockState = {
      userSettings: { ...baseSettings(), defaultUtilityAgentId: "legacy-agent-1" },
    };

    expect(useIsUtilityConfigured()).toBe(true);
  });

  it("is not configured when neither field is set", () => {
    mockState = { userSettings: baseSettings() };

    expect(useIsUtilityConfigured()).toBe(false);
  });
});
