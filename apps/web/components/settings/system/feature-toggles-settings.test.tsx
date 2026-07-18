import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeFlagState } from "@/lib/types/runtime-flags";
import { FeatureTogglesSettings } from "./feature-toggles-settings";

const fetchRuntimeFlagsMock = vi.fn();
const toastMock = vi.fn();
const DEBUG_MODE_LABEL = "Debug mode";
const FEATURE_TOGGLES_LOAD_FAILURE = "Feature toggles could not be loaded.";

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
  useToast: () => ({ toast: toastMock }),
}));

vi.mock("@/lib/api/domains/runtime-flags-api", () => ({
  fetchRuntimeFlags: (...args: unknown[]) => fetchRuntimeFlagsMock(...args),
  updateRuntimeFlag: vi.fn(),
}));

beforeEach(() => {
  fetchRuntimeFlagsMock.mockReset();
  toastMock.mockReset();
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("browser demo action", () => {
  it("offers the browser demo in a separate tab during debug development", () => {
    render(
      <TooltipProvider>
        <FeatureTogglesSettings
          initialFlags={[flagState({ requires_restart_to_apply: false })]}
          restartCapability={null}
          browserDemoAvailable
        />
      </TooltipProvider>,
    );

    const link = screen.getByRole("link", { name: "Open browser demo" });
    expect(link.getAttribute("href")).toBe("/demo");
    expect(link.getAttribute("target")).toBe("_blank");
  });
});

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

  it("automatically reloads when the initial runtime flags payload is empty", async () => {
    fetchRuntimeFlagsMock.mockResolvedValueOnce({
      flags: [flagState({ requires_restart_to_apply: false })],
    });

    render(
      <TooltipProvider>
        <FeatureTogglesSettings initialFlags={[]} restartCapability={null} />
      </TooltipProvider>,
    );

    await waitFor(() => expect(fetchRuntimeFlagsMock).toHaveBeenCalledTimes(1));
    expect(await screen.findByText(DEBUG_MODE_LABEL)).not.toBeNull();
    expect(screen.queryByText(FEATURE_TOGGLES_LOAD_FAILURE)).toBeNull();
  });

  it("shows a loading state while the empty initial runtime flags payload reloads", async () => {
    let resolveFlags: (value: { flags: RuntimeFlagState[] }) => void = () => {};
    fetchRuntimeFlagsMock.mockReturnValueOnce(
      new Promise<{ flags: RuntimeFlagState[] }>((resolve) => {
        resolveFlags = resolve;
      }),
    );

    render(
      <TooltipProvider>
        <FeatureTogglesSettings initialFlags={[]} restartCapability={null} />
      </TooltipProvider>,
    );

    await waitFor(() => expect(fetchRuntimeFlagsMock).toHaveBeenCalledTimes(1));
    expect(screen.getByText("Loading feature toggles...")).not.toBeNull();
    expect(screen.queryByText(FEATURE_TOGGLES_LOAD_FAILURE)).toBeNull();

    resolveFlags({ flags: [flagState({ requires_restart_to_apply: false })] });

    expect(await screen.findByText(DEBUG_MODE_LABEL)).not.toBeNull();
  });

  it("keeps the retry state and shows a toast when the empty initial reload fails", async () => {
    fetchRuntimeFlagsMock.mockRejectedValueOnce(new Error("boom"));

    render(
      <TooltipProvider>
        <FeatureTogglesSettings initialFlags={[]} restartCapability={null} />
      </TooltipProvider>,
    );

    expect(await screen.findByText(FEATURE_TOGGLES_LOAD_FAILURE)).not.toBeNull();
    expect(screen.getByRole("button", { name: "Retry" })).not.toBeNull();
    expect(toastMock).toHaveBeenCalledWith({
      title: "Failed to load feature toggles",
      description: "boom",
      variant: "error",
    });
  });

  it("keeps the retry state without a toast when the empty initial reload returns no flags", async () => {
    fetchRuntimeFlagsMock.mockResolvedValueOnce({ flags: [] });

    render(
      <TooltipProvider>
        <FeatureTogglesSettings initialFlags={[]} restartCapability={null} />
      </TooltipProvider>,
    );

    expect(await screen.findByText(FEATURE_TOGGLES_LOAD_FAILURE)).not.toBeNull();
    expect(screen.getByRole("button", { name: "Retry" })).not.toBeNull();
    expect(toastMock).not.toHaveBeenCalled();
  });

  it("deduplicates the empty initial reload across remounts while the request is in flight", async () => {
    let resolveFlags: (value: { flags: RuntimeFlagState[] }) => void = () => {};
    fetchRuntimeFlagsMock.mockReturnValueOnce(
      new Promise<{ flags: RuntimeFlagState[] }>((resolve) => {
        resolveFlags = resolve;
      }),
    );

    const firstRender = render(
      <TooltipProvider>
        <FeatureTogglesSettings initialFlags={[]} restartCapability={null} />
      </TooltipProvider>,
    );
    await waitFor(() => expect(fetchRuntimeFlagsMock).toHaveBeenCalledTimes(1));
    firstRender.unmount();

    render(
      <TooltipProvider>
        <FeatureTogglesSettings initialFlags={[]} restartCapability={null} />
      </TooltipProvider>,
    );
    expect(fetchRuntimeFlagsMock).toHaveBeenCalledTimes(1);

    resolveFlags({ flags: [flagState({ requires_restart_to_apply: false })] });

    expect(await screen.findByText(DEBUG_MODE_LABEL)).not.toBeNull();
  });
});

function flagState(overrides: Partial<RuntimeFlagState> = {}): RuntimeFlagState {
  return {
    key: "debug.devMode",
    env_var: "KANDEV_DEBUG_DEV_MODE",
    label: DEBUG_MODE_LABEL,
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
