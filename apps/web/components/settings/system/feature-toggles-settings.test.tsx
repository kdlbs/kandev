import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RuntimeFlagState } from "@/lib/types/runtime-flags";
import { FeatureTogglesSettings } from "./feature-toggles-settings";

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

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

afterEach(cleanup);

describe("FeatureTogglesSettings", () => {
  it("shows restart support details without offering restart when unsupported", () => {
    render(
      <TooltipProvider>
        <FeatureTogglesSettings
          initialFlags={[flagState()]}
          restartCapability={{
            supported: false,
            mode: "manual",
            reason: "Automatic restart is not available for this launch mode.",
          }}
        />
      </TooltipProvider>,
    );

    expect(screen.getByText("Restart required")).not.toBeNull();
    expect(screen.getByLabelText("Restart support details")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Restart" })).toBeNull();
    expect(screen.getByText(/terminal or service manager/)).not.toBeNull();
  });
});

function flagState(overrides: Partial<RuntimeFlagState> = {}): RuntimeFlagState {
  return {
    key: "debug.devMode",
    env_var: "KANDEV_DEBUG_DEV_MODE",
    label: "Debug mode",
    description: "Enables diagnostic tools for troubleshooting.",
    kind: "debug",
    stability: "stable",
    risk_level: "high",
    risk_description: "Use only on trusted machines.",
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
