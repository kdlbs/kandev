import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { RuntimeFlagState } from "@/lib/types/runtime-flags";
import { renderSettingsRoute } from "./settings-routes";

// Route-level coverage. The component-level suite in
// components/settings/system/feature-toggles-settings.test.tsx cannot catch a
// route that supplies the wrong prop, which is exactly how the restart action
// was lost in #1389.
const mocks = vi.hoisted(() => ({
  fetchRestartCapability: vi.fn(),
  fetchRuntimeFlags: vi.fn(),
  updateRuntimeFlag: vi.fn(),
  fetchSystemInfo: vi.fn(),
  requestRestart: vi.fn(),
}));

vi.mock("@/lib/api/domains/system-api", () => ({
  fetchRestartCapability: mocks.fetchRestartCapability,
  fetchSystemInfo: mocks.fetchSystemInfo,
  requestRestart: mocks.requestRestart,
}));

vi.mock("@/lib/api/domains/runtime-flags-api", () => ({
  fetchRuntimeFlags: mocks.fetchRuntimeFlags,
  updateRuntimeFlag: mocks.updateRuntimeFlag,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: () => {},
}));

const PENDING_RESTART_FLAG: RuntimeFlagState = {
  key: "features.office",
  kind: "feature",
  label: "Office mode",
  description: "Enables autonomous agent office workflows and related settings.",
  stability: "experimental",
  risk_level: "medium",
  risk_description: "Office mode is still evolving.",
  effective_value: false,
  default_value: false,
  override_value: true,
  source: "override",
  env_var: "KANDEV_FEATURES_OFFICE",
  env_locked: false,
  restart_required: true,
  requires_restart_to_apply: true,
  mutable: true,
};

function renderFeatureTogglesRoute() {
  return render(
    <TooltipProvider>{renderSettingsRoute("/settings/system/feature-toggles")}</TooltipProvider>,
  );
}

beforeEach(() => {
  mocks.fetchRestartCapability.mockReset();
  mocks.fetchRuntimeFlags.mockReset();
  mocks.fetchRuntimeFlags.mockResolvedValue({ flags: [PENDING_RESTART_FLAG] });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("feature toggles route restart capability", () => {
  it("offers the in-app restart action when the running backend supports restart", async () => {
    mocks.fetchRestartCapability.mockResolvedValue({
      supported: true,
      mode: "supervisor",
      adapter: "supervisor",
    });

    renderFeatureTogglesRoute();

    expect(await screen.findByRole("button", { name: "Restart" })).not.toBeNull();
    expect(screen.queryByText(/terminal or service manager/)).toBeNull();
  });

  it("falls back to manual guidance when the running backend cannot restart", async () => {
    mocks.fetchRestartCapability.mockResolvedValue({
      supported: false,
      mode: "manual",
      reason: "Automatic restart is not available for this launch mode.",
    });

    renderFeatureTogglesRoute();

    expect(await screen.findByText(/terminal or service manager/)).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Restart" })).toBeNull();
  });

  it("fails closed to manual guidance when the capability request rejects", async () => {
    mocks.fetchRestartCapability.mockRejectedValue(new Error("network down"));

    renderFeatureTogglesRoute();

    expect(await screen.findByText(/terminal or service manager/)).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Restart" })).toBeNull();
  });

  it("renders the toggle cards without waiting for the capability request", async () => {
    let resolveCapability: (value: unknown) => void = () => {};
    mocks.fetchRestartCapability.mockReturnValue(
      new Promise((resolve) => {
        resolveCapability = resolve;
      }),
    );

    renderFeatureTogglesRoute();

    // Cards are reachable while the capability request is still in flight.
    expect(await screen.findByTestId("feature-toggles-settings")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Restart" })).toBeNull();

    resolveCapability({ supported: true, mode: "supervisor" });

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Restart" })).not.toBeNull();
    });
  });
});
