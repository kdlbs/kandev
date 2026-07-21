import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { defaultSystemState } from "@/lib/state/slices/system/system-slice";
import type { AppState } from "@/lib/state/store";
import { StatusSurfaceMetrics } from "./status-surface-metrics";

const responsiveState = vi.hoisted(() => ({ isMobile: false }));
const subscribeMock = vi.hoisted(() => vi.fn());

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

function renderMetrics(drawerOpen = false) {
  return render(
    <StateProvider
      initialState={
        {
          userSettings: {
            ...defaultSettingsState.userSettings,
            loaded: true,
            systemMetricsDisplay: { showInTopbar: true },
          },
          system: {
            ...defaultSystemState.system,
            metrics: {
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
                    {
                      id: "cpu_temp",
                      label: "CPU temperature",
                      unit: "°C",
                      value: 72,
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

  it("subscribes and renders in the desktop status bar", () => {
    renderMetrics();

    expect(subscribeMock).toHaveBeenCalledWith(true);
    expect(screen.getByTestId("app-status-metrics")).toBeTruthy();
    expect(screen.getByLabelText("CPU 42%")).toBeTruthy();
    expect(screen.getByLabelText("Memory 51%")).toBeTruthy();
    expect(screen.getByLabelText("Disk 63%")).toBeTruthy();
    expect(screen.getByLabelText("CPU temperature 72°C")).toBeTruthy();
    expect(screen.queryByLabelText("Executor metrics")).toBeNull();
    expect(screen.queryByLabelText("Memory 99%")).toBeNull();
  });

  it("does not subscribe or render while the phone Status drawer is closed", () => {
    responsiveState.isMobile = true;
    renderMetrics(false);

    expect(subscribeMock).toHaveBeenCalledWith(false);
    expect(screen.queryByTestId("app-status-metrics")).toBeNull();
  });

  it("subscribes and renders rows when the phone Status drawer opens", () => {
    responsiveState.isMobile = true;
    renderMetrics(true);

    expect(subscribeMock).toHaveBeenCalledWith(true);
    expect(screen.getByLabelText("Host metrics").parentElement?.className).toContain("min-h-11");
  });
});
