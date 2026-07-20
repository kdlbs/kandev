import { cleanup, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import type { AppState } from "@/lib/state/store";
import type { SystemMetricsSnapshot } from "@/lib/types/system";
import { TopbarMetrics } from "./topbar-metrics";

const responsiveState = vi.hoisted(() => ({ isMobile: true }));
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

function renderMetrics(
  initialState: Partial<AppState>,
  metrics: SystemMetricsSnapshot,
  activeSessionId?: string | null,
) {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(qk.system.metrics(), metrics);
  return render(
    <QueryClientProvider client={queryClient}>
      <StateProvider initialState={initialState}>
        <TooltipProvider>
          <TopbarMetrics activeSessionId={activeSessionId} />
        </TooltipProvider>
      </StateProvider>
    </QueryClientProvider>,
  );
}

function initialState(showInTopbar: boolean): Partial<AppState> {
  return {
    userSettings: {
      ...defaultSettingsState.userSettings,
      loaded: true,
      systemMetricsDisplay: { showInTopbar },
    },
  };
}

function metricsSnapshot(): SystemMetricsSnapshot {
  return {
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
            value: 68,
            available: true,
          },
          {
            id: "disk_percent",
            label: "Disk",
            unit: "%",
            value: 12,
            available: true,
          },
        ],
      },
      {
        id: "execution-session-1",
        label: "Executor",
        kind: "execution",
        session_id: "session-1",
        metrics: [
          {
            id: "cpu_percent",
            label: "CPU",
            unit: "%",
            value: 17,
            available: true,
          },
          {
            id: "memory_percent",
            label: "Memory",
            unit: "%",
            value: 33,
            available: true,
          },
        ],
      },
    ],
  };
}

describe("TopbarMetrics", () => {
  beforeEach(() => {
    responsiveState.isMobile = true;
    subscribeMock.mockClear();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders a compact metrics pill on mobile when enabled", () => {
    renderMetrics(initialState(true), metricsSnapshot());

    const metrics = screen.getByTestId("mobile-topbar-metrics");
    expect(metrics).toBeTruthy();
    expect(metrics.className.split(/\s+/)).not.toContain("border");
    expect(metrics.className.split(/\s+/)).not.toContain("border-border");
    expect(screen.getByLabelText("CPU 42%")).toBeTruthy();
    expect(screen.getByLabelText("Memory 68%")).toBeTruthy();
    expect(screen.queryByLabelText("Disk 12%")).toBeNull();
    expect(subscribeMock).toHaveBeenCalledWith(true);
  });

  it("prefers the active executor source in compact task topbars", () => {
    renderMetrics(initialState(true), metricsSnapshot(), "session-1");

    expect(screen.getByLabelText("Executor metrics")).toBeTruthy();
    expect(screen.getByLabelText("CPU 17%")).toBeTruthy();
    expect(screen.getByLabelText("Memory 33%")).toBeTruthy();
    expect(screen.queryByLabelText("CPU 42%")).toBeNull();
  });

  it("does not subscribe or render when topbar metrics are disabled", () => {
    renderMetrics(initialState(false), metricsSnapshot());

    expect(screen.queryByTestId("mobile-topbar-metrics")).toBeNull();
    expect(subscribeMock).toHaveBeenCalledWith(false);
  });
});
