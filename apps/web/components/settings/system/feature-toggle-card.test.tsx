import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RuntimeFlagState } from "@/lib/types/runtime-flags";
import { FeatureToggleCard } from "./feature-toggle-card";

vi.mock("@kandev/ui/switch", () => ({
  Switch: ({
    checked,
    disabled,
    "aria-label": ariaLabel,
    onCheckedChange,
  }: {
    checked: boolean;
    disabled: boolean;
    "aria-label": string;
    onCheckedChange: (checked: boolean) => void;
  }) => (
    <button
      aria-label={ariaLabel}
      aria-pressed={checked}
      disabled={disabled}
      type="button"
      onClick={() => onCheckedChange(!checked)}
    />
  ),
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

  it("calls onChange with the next switch value", () => {
    const onChange = vi.fn();
    render(
      <FeatureToggleCard
        flag={flagState({ effective_value: false })}
        saving={false}
        onChange={onChange}
        onReset={() => undefined}
      />,
    );

    fireEvent.click(screen.getByLabelText("Toggle Office mode"));

    expect(onChange).toHaveBeenCalledWith(true);
  });

  it("calls onReset from the use default action when an override exists", () => {
    const onReset = vi.fn();
    render(
      <FeatureToggleCard
        flag={flagState({ override_value: false })}
        saving={false}
        onChange={() => undefined}
        onReset={onReset}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /use default/i }));

    expect(onReset).toHaveBeenCalledTimes(1);
  });

  it("disables changes and reset when the launch environment controls the flag", () => {
    const onChange = vi.fn();
    const onReset = vi.fn();
    render(
      <FeatureToggleCard
        flag={flagState({ env_locked: true, source: "env" })}
        saving={false}
        onChange={onChange}
        onReset={onReset}
      />,
    );

    expect(screen.getByLabelText("Toggle Office mode")).toHaveProperty("disabled", true);
    expect(screen.getByRole("button", { name: /use default/i })).toHaveProperty("disabled", true);
    expect(screen.getByText("Controlled by launch environment")).not.toBeNull();
  });

  it("shows restart metadata when a saved change is pending", () => {
    render(
      <FeatureToggleCard
        flag={flagState({ restart_required: true, requires_restart_to_apply: true })}
        saving={false}
        onChange={() => undefined}
        onReset={() => undefined}
      />,
    );

    expect(screen.getByText("Requires restart")).not.toBeNull();
    expect(screen.getByText("Pending restart")).not.toBeNull();
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
