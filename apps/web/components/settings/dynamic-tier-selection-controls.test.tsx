import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DynamicAgentCandidate } from "@/lib/types/agent-profile";
import { DynamicTierSelectionControls, withCandidateCost } from "./dynamic-tier-selection-controls";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

function candidate(selection: Record<string, unknown>): DynamicAgentCandidate {
  return {
    position: 0,
    executionProfileId: "profile-1",
    enabled: true,
    policies: { selection } as unknown as DynamicAgentCandidate["policies"],
  };
}

describe("dynamic tier selection controls", () => {
  afterEach(cleanup);

  it("sends an explicit null so Unknown clears a saved cost", () => {
    const next = withCandidateCost(candidate({ model: { cost: "free", extra: 1 } }), "");
    expect(next.policies.selection?.model).toEqual({ cost: null, extra: 1 });
  });

  it("disables usage modes without provider usage limits and keeps a saved one", () => {
    render(
      <DynamicTierSelectionControls
        candidate={candidate({ tier: { mode: "pace" } })}
        head
        onChange={() => {}}
      />,
    );
    const mode = screen.getByLabelText("agents:tierSelectionMode") as HTMLSelectElement;
    expect(mode.value).toBe("pace");
    const disabled = Array.from(mode.options)
      .filter((option) => option.disabled)
      .map((option) => option.value);
    expect(disabled).toEqual(["headroom", "pace", "consumption_rate"]);
    expect(screen.getByText("agents:tierUsageModesUnavailable")).toBeTruthy();
  });
});
