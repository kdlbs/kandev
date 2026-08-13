import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { UtilityAgent } from "@/lib/api/domains/utility-api";
import { BuiltinActionRow, USE_DEFAULT } from "./utility-sections";

const UNCONFIGURED = "__UNCONFIGURED__";

vi.mock("./utility-agent-profile-picker", () => ({
  UtilityAgentProfilePicker: ({
    testId,
    value,
    unavailableValue,
  }: {
    testId: string;
    value: string;
    unavailableValue?: string;
  }) => <div data-testid={testId} data-value={value} data-unavailable-value={unavailableValue} />,
  utilityProfileEligibility: () => true,
}));

afterEach(() => cleanup());

function builtin(overrides: Partial<UtilityAgent> = {}): UtilityAgent {
  return {
    id: "builtin-commit-message",
    name: "commit-message",
    description: "Generate a commit message.",
    prompt: "Generate a commit message.",
    agent_id: "",
    model: "",
    builtin: true,
    enabled: true,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function renderRow(agent: UtilityAgent) {
  return render(
    <BuiltinActionRow
      agent={agent}
      profiles={[]}
      defaultLabel="Default profile"
      onProfileChange={vi.fn()}
      onEdit={vi.fn()}
      isDirty={false}
    />,
  );
}

describe("BuiltinActionRow profile binding state", () => {
  it("uses the default for an inherited built-in binding", () => {
    renderRow(builtin({ profile_binding_state: "inherit", agent_profile_id: "" }));

    const picker = screen.getByTestId("utility-profile-picker-action-builtin-commit-message");
    expect(picker.getAttribute("data-value")).toBe(USE_DEFAULT);
    expect(picker.getAttribute("data-unavailable-value")).toBeNull();
  });

  it("keeps an empty unconfigured binding unavailable", () => {
    renderRow(builtin({ profile_binding_state: "unconfigured", agent_profile_id: "" }));

    const picker = screen.getByTestId("utility-profile-picker-action-builtin-commit-message");
    expect(picker.getAttribute("data-value")).toBe(UNCONFIGURED);
    expect(picker.getAttribute("data-unavailable-value")).toBe(UNCONFIGURED);
  });

  it("keeps a concrete unconfigured binding unavailable", () => {
    renderRow(
      builtin({
        profile_binding_state: "unconfigured",
        agent_profile_id: "deleted-profile",
      }),
    );

    const picker = screen.getByTestId("utility-profile-picker-action-builtin-commit-message");
    expect(picker.getAttribute("data-value")).toBe("deleted-profile");
    expect(picker.getAttribute("data-unavailable-value")).toBe("deleted-profile");
  });
});
