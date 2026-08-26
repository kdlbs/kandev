import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { defaultSystemState } from "@/lib/state/slices/system/system-slice";
import type { AppState } from "@/lib/state/store";
import { StatusSurfaceMetrics } from "./status-surface-metrics";

const responsiveState = vi.hoisted(() => ({ isMobile: false }));
const subscribeMock = vi.hoisted(() => vi.fn());
const metricsTestId = "app-status-metrics";
const cpuAccessibleName = "CPU 42%";
const memoryAccessibleName = "Memory 51%";
const allHostMetricAccessibleNames = [
  cpuAccessibleName,
  memoryAccessibleName,
  "Disk 63%",
  "System load (1 min) 2.5",
  "CPU temperature 71°C",
  "network_rx 12.3MB/s",
];

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: responsiveState.isMobile ? "mobile" : "desktop",
    isMobile: responsiveState.isMobile,
    isTablet: false,
    isDesktop: !responsiveState.isMobile,
    isCompactDesktop: false,
    isFullDesktop: !responsiveState.isMobile,
    isFinePointer: !responsiveState.isMobile,
    usesDesktopWorkbench: !responsiveState.isMobile,
  }),
}));

vi.mock("@/hooks/use-system-metrics-subscription", () => ({
  useSystemMetricsSubscription: subscribeMock,
}));

function renderMetrics(drawerOpen = false, simplified = false, loading = false) {
  return render(
    <StateProvider
      initialState={
        {
          userSettings: {
            ...defaultSettingsState.userSettings,
            loaded: true,
            systemMetricsDisplay: { showInTopbar: true, simplified },
          },
          system: {
            ...defaultSystemState.system,
            metrics: loading
              ? null
              : {
                  timestamp: "2026-06-23T10:00:00Z",
                  interval_seconds: 5,
                  sources: [
                    {
                      id: "backend",
                      label: "Host",
                      kind: "backend",
                      metrics: [
                        {
                          id: "cpu_percent",
                          label: "CPU",
                          unit: "%",
                          value: 42,
                          available: true,
                        },
                        {
                          id: "memory_percent",
                          label: "Memory",
                          unit: "%",
                          value: 51,
                          available: true,
                        },
                        {
                          id: "disk_percent",
                          label: "Disk",
                          unit: "%",
                          value: 63,
                          available: true,
                        },
                        { id: "io_load", label: "Load average", value: 2.5, available: true },
                        {
                          id: "cpu_temp",
                          label: "CPU temperature",
                          unit: "°C",
                          value: 71,
                          available: true,
                        },
                        {
                          id: "network_rx",
                          label: "Network receive",
                          unit: "MB/s",
                          value: 12.3,
                          available: true,
                        },
                      ],
                    },
                    {
                      id: "executor-session-1",
                      label: "Demo executor",
                      kind: "execution",
                      session_id: "session-1",
                      metrics: [
                        {
                          id: "memory_percent",
                          label: "Memory",
                          unit: "%",
                          value: 99,
                          available: true,
                        },
                      ],
                    },
                  ],
                },
          },
        } satisfies Partial<AppState>
      }
    >
      <TooltipProvider>
        <StatusSurfaceMetrics
          presentation={responsiveState.isMobile ? "mobile-drawer" : "bar"}
          density="full"
          drawerOpen={drawerOpen}
        />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("StatusSurfaceMetrics", () => {
  beforeEach(() => {
    responsiveState.isMobile = false;
    subscribeMock.mockClear();
  });

  afterEach(cleanup);

  it("subscribes and renders in the desktop status bar", async () => {
    renderMetrics();

    expect(subscribeMock).toHaveBeenCalledWith(true);
    expect(screen.getByTestId(metricsTestId)).toBeTruthy();
    expect(screen.getByLabelText(cpuAccessibleName)).toBeTruthy();
    expect(screen.getByLabelText(memoryAccessibleName)).toBeTruthy();
    expect(screen.getByLabelText("Disk 63%")).toBeTruthy();
    expect(screen.getAllByTestId("system-metric-meter")).toHaveLength(3);
    const systemLoad = screen.getByLabelText("System load (1 min) 2.5");
    expect(systemLoad).toBeTruthy();
    fireEvent.focus(systemLoad);
    expect(
      await screen.findAllByText(
        "Average number of tasks running or waiting for CPU during the last minute. Compare this value with the host's CPU core count.",
      ),
    ).not.toHaveLength(0);
    expect(screen.queryByLabelText("Executor metrics")).toBeNull();
    expect(screen.queryByLabelText("Memory 99%")).toBeNull();
    expect(screen.queryByLabelText("CPU temperature 71°C")).toBeNull();
    expect(screen.queryByLabelText("network_rx 12.3MB/s")).toBeNull();
  });

  it("keeps the desktop metrics item hidden while the first snapshot is loading", () => {
    renderMetrics(false, false, true);

    expect(subscribeMock).toHaveBeenCalledWith(true);
    expect(screen.queryByTestId(metricsTestId)).toBeNull();
    expect(screen.queryByText("Metrics unavailable")).toBeNull();
  });

  it("does not subscribe or render while the phone Status drawer is closed", () => {
    responsiveState.isMobile = true;
    renderMetrics(false);

    expect(subscribeMock).toHaveBeenCalledWith(false);
    expect(screen.queryByTestId(metricsTestId)).toBeNull();
  });

  it("subscribes and renders rows when the phone Status drawer opens", () => {
    responsiveState.isMobile = true;
    renderMetrics(true);

    expect(subscribeMock).toHaveBeenCalledWith(true);
    const metricsRow = screen.getByLabelText("Host metrics").parentElement;
    expect(metricsRow?.className).toContain("min-h-11");
    expect(metricsRow?.className).toContain("w-full");
    expect(metricsRow?.className).toContain("min-w-0");
  });

  it("renders every received host metric in the phone Status drawer", () => {
    responsiveState.isMobile = true;
    renderMetrics(true);

    expect(screen.getByLabelText(cpuAccessibleName).parentElement?.className).toContain("grid");
    for (const accessibleName of allHostMetricAccessibleNames) {
      expect(screen.getByLabelText(accessibleName)).toBeTruthy();
    }
  });

  it("keeps the phone metrics row hidden while the first snapshot is loading", () => {
    responsiveState.isMobile = true;
    renderMetrics(true, false, true);

    expect(subscribeMock).toHaveBeenCalledWith(true);
    expect(screen.queryByTestId(metricsTestId)).toBeNull();
    expect(screen.queryByText("Metrics unavailable")).toBeNull();
  });

  it("omits the host marker and meters in simplified desktop mode", () => {
    renderMetrics(false, true);

    expect(screen.queryByLabelText("Host metrics")).toBeNull();
    expect(screen.queryAllByTestId("system-metric-meter")).toHaveLength(0);
    expect(screen.getByLabelText(cpuAccessibleName)).toBeTruthy();
    expect(screen.getByLabelText(memoryAccessibleName)).toBeTruthy();
  });

  it("omits the host marker and meters in simplified phone drawer mode", () => {
    responsiveState.isMobile = true;
    renderMetrics(true, true);

    expect(screen.queryByLabelText("Host metrics")).toBeNull();
    expect(screen.queryAllByTestId("system-metric-meter")).toHaveLength(0);
    for (const accessibleName of allHostMetricAccessibleNames) {
      expect(screen.getByLabelText(accessibleName)).toBeTruthy();
    }
  });
});
