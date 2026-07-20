import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { RuntimeFlagState } from "@/lib/types/runtime-flags";
import type { SettingsSaveContributor } from "../settings-save-provider";
import { FeatureTogglesSettings } from "./feature-toggles-settings";

const fetchRuntimeFlagsMock = vi.fn();
const updateRuntimeFlagMock = vi.fn();
const toastMock = vi.fn();
let saveContributor: SettingsSaveContributor | null = null;
const DEBUG_MODE_LABEL = "Debug mode";
const FEATURE_TOGGLES_LOAD_FAILURE = "Feature toggles could not be loaded.";

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

vi.mock("../settings-save-provider", () => ({
  useSettingsSaveContributor: (contributor: SettingsSaveContributor) => {
    saveContributor = contributor;
  },
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

vi.mock("@/lib/api/domains/runtime-flags-api", () => ({
  fetchRuntimeFlags: (...args: unknown[]) => fetchRuntimeFlagsMock(...args),
  updateRuntimeFlag: (...args: unknown[]) => updateRuntimeFlagMock(...args),
}));

beforeEach(() => {
  fetchRuntimeFlagsMock.mockReset();
  updateRuntimeFlagMock.mockReset();
  toastMock.mockReset();
  saveContributor = null;
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function createQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

function renderFeatureToggles(
  props: React.ComponentProps<typeof FeatureTogglesSettings>,
  queryClient = createQueryClient(),
) {
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <FeatureTogglesSettings {...props} />
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

describe("FeatureTogglesSettings", () => {
  it("stages an override until the route save contributor runs", async () => {
    const initial = flagState({
      override_value: null,
      effective_value: false,
      source: "default",
      requires_restart_to_apply: false,
    });
    const persisted = flagState({ override_value: true, effective_value: true });
    updateRuntimeFlagMock.mockResolvedValueOnce({ flags: [persisted] });

    renderFeatureToggles({ initialFlags: [initial], restartCapability: null });

    fireEvent.click(screen.getByRole("button", { name: `Toggle ${DEBUG_MODE_LABEL}` }));

    expect(updateRuntimeFlagMock).not.toHaveBeenCalled();
    expect(saveContributor?.isDirty).toBe(true);
    expect(
      screen.getByTestId(`feature-toggle-${initial.key}`).getAttribute("data-settings-dirty"),
    ).toBe("true");
    if (!saveContributor) throw new Error("Save contributor was not registered");

    await saveContributor.save(saveContributor.revision);

    expect(updateRuntimeFlagMock).toHaveBeenCalledWith(initial.key, true);
  });

  it("shows restart support details without offering restart when unsupported", () => {
    renderFeatureToggles({
      initialFlags: [flagState()],
      restartCapability: {
        supported: false,
        mode: "manual",
        reason: "Automatic restart is not available for this launch mode.",
      },
    });

    expect(screen.getByText("Restart required")).not.toBeNull();
    expect(screen.getByLabelText("Restart support details")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Restart" })).toBeNull();
    expect(screen.getByText(/terminal or service manager/)).not.toBeNull();
  });

  it("automatically reloads when the initial runtime flags payload is empty", async () => {
    fetchRuntimeFlagsMock.mockResolvedValueOnce({
      flags: [flagState({ requires_restart_to_apply: false })],
    });

    renderFeatureToggles({ initialFlags: [], restartCapability: null });

    await waitFor(() => expect(fetchRuntimeFlagsMock).toHaveBeenCalledTimes(1));
    expect(await screen.findByText(DEBUG_MODE_LABEL)).not.toBeNull();
    expect(screen.queryByText(FEATURE_TOGGLES_LOAD_FAILURE)).toBeNull();
  });
});

describe("FeatureTogglesSettings bootstrap reload", () => {
  it("shows a loading state while the empty initial runtime flags payload reloads", async () => {
    let resolveFlags: (value: { flags: RuntimeFlagState[] }) => void = () => {};
    fetchRuntimeFlagsMock.mockReturnValueOnce(
      new Promise<{ flags: RuntimeFlagState[] }>((resolve) => {
        resolveFlags = resolve;
      }),
    );

    renderFeatureToggles({ initialFlags: [], restartCapability: null });

    await waitFor(() => expect(fetchRuntimeFlagsMock).toHaveBeenCalledTimes(1));
    expect(screen.getByText("Loading feature toggles...")).not.toBeNull();
    expect(screen.queryByText(FEATURE_TOGGLES_LOAD_FAILURE)).toBeNull();

    resolveFlags({ flags: [flagState({ requires_restart_to_apply: false })] });

    expect(await screen.findByText(DEBUG_MODE_LABEL)).not.toBeNull();
  });

  it("keeps the retry state and shows a toast when the empty initial reload fails", async () => {
    fetchRuntimeFlagsMock.mockRejectedValueOnce(new Error("boom"));

    renderFeatureToggles({ initialFlags: [], restartCapability: null });

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

    renderFeatureToggles({ initialFlags: [], restartCapability: null });

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

    const queryClient = createQueryClient();
    const firstRender = renderFeatureToggles(
      { initialFlags: [], restartCapability: null },
      queryClient,
    );
    await waitFor(() => expect(fetchRuntimeFlagsMock).toHaveBeenCalledTimes(1));
    firstRender.unmount();

    renderFeatureToggles({ initialFlags: [], restartCapability: null }, queryClient);
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
