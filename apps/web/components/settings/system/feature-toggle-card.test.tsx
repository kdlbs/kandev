import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RuntimeFlagState } from "@/lib/types/runtime-flags";
import { FeatureToggleCard } from "./feature-toggle-card";

vi.mock("@kandev/ui/switch", () => ({
  Switch: ({
    checked,
    disabled,
    "aria-label": ariaLabel,
  }: {
    checked: boolean;
    disabled: boolean;
    "aria-label": string;
  }) => <button aria-label={ariaLabel} aria-pressed={checked} disabled={disabled} type="button" />,
}));

afterEach(cleanup);

describe("FeatureToggleCard", () => {
  it("shows risk copy as supporting text instead of a warning alert", () => {
    render(
      <FeatureToggleCard
        flag={flagState({
          risk_description:
            "Office mode is still evolving and should be reviewed before relying on it.",
        })}
        saving={false}
        onChange={() => undefined}
        onReset={() => undefined}
      />,
    );

    expect(screen.getByText(/Office mode is still evolving/)).not.toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

function flagState(overrides: Partial<RuntimeFlagState> = {}): RuntimeFlagState {
  return {
    key: "features.office",
    env_var: "KANDEV_FEATURES_OFFICE",
    label: "Office mode",
    description: "Enables autonomous agent office workflows and related settings.",
    kind: "feature",
    stability: "experimental",
    risk_level: "medium",
    risk_description: "",
    default_value: false,
    override_value: true,
    effective_value: true,
    source: "override",
    env_locked: false,
    restart_required: true,
    requires_restart_to_apply: true,
    mutable: true,
    ...overrides,
  };
}
