import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SystemMetricsGlobalSettings } from "@/lib/types/system";
import { SettingsSaveProvider } from "./settings-save-provider";
import { SystemMetricsSettingsCard } from "./system-metrics-settings-card";

const settings: SystemMetricsGlobalSettings = {
  metrics: ["cpu_percent", "memory_percent", "disk_percent"],
  interval_seconds: 5,
  backend_disk_path: "/",
  collect_execution: false,
};
const updateSystemMetricsSettingsMock = vi.fn();
const DIRTY_ATTRIBUTE = "data-settings-dirty";

vi.mock("@/lib/api", () => ({
  fetchSystemMetricsSettings: vi.fn(async () => ({ settings })),
  updateSystemMetricsSettings: (...args: unknown[]) => updateSystemMetricsSettingsMock(...args),
}));

afterEach(() => {
  cleanup();
  updateSystemMetricsSettingsMock.mockReset();
});

describe("SystemMetricsSettingsCard", () => {
  it("keeps metric changes local until Save changes is pressed", async () => {
    updateSystemMetricsSettingsMock.mockImplementation(async (next) => ({ settings: next }));
    render(
      <SettingsSaveProvider>
        <SystemMetricsSettingsCard showInTopbar onShowInTopbarChange={vi.fn()} />
      </SettingsSaveProvider>,
    );

    const cpuMetric = await screen.findByRole("checkbox", { name: "CPU %" });
    await waitFor(() => expect(cpuMetric.getAttribute("data-state")).toBe("checked"));
    fireEvent.click(cpuMetric);

    expect(updateSystemMetricsSettingsMock).not.toHaveBeenCalled();
    expect(cpuMetric.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
    expect(cpuMetric.closest('[data-slot="card"]')?.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(updateSystemMetricsSettingsMock).toHaveBeenCalledTimes(1));
    expect(updateSystemMetricsSettingsMock.mock.calls[0]?.[0].metrics).not.toContain("cpu_percent");
    await waitFor(() => expect(cpuMetric.getAttribute(DIRTY_ATTRIBUTE)).toBe("false"));
    expect(cpuMetric.closest('[data-slot="card"]')?.getAttribute(DIRTY_ATTRIBUTE)).toBe("false");
  });
});
