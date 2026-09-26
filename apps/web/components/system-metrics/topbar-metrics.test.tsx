import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { defaultSystemState } from "@/lib/state/slices/system/system-slice";
import type { AppState } from "@/lib/state/store";
import { TopbarMetrics } from "./topbar-metrics";

const subscribeMock = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/use-system-metrics-subscription", () => ({
  useSystemMetricsSubscription: subscribeMock,
}));

function renderMetrics(appStatusBarEnabled: boolean, mobile = false) {
  const initialState = {
    userSettings: {
      ...defaultSettingsState.userSettings,
      appStatusBarEnabled,
      systemMetricsDisplay: { showInTopbar: true, simplified: false },
    },
    system: {
      ...defaultSystemState.system,
      metrics: {
        timestamp: "2026-07-22T10:00:00Z",
        interval_seconds: 5,
        sources: [
          {
            id: "backend",
            label: "Host",
            kind: "backend",
            metrics: [{ id: "cpu_percent", label: "CPU", unit: "%", value: 42, available: true }],
          },
        ],
      },
    },
  } satisfies Partial<AppState>;

  return render(
    <StateProvider initialState={initialState}>
      <TooltipProvider>
        <TopbarMetrics mobile={mobile} />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("TopbarMetrics preference fallback", () => {
  beforeEach(() => {
    subscribeMock.mockClear();
  });

  afterEach(cleanup);

  it("keeps existing metrics visible while the app status bar is disabled", () => {
    renderMetrics(false);

    expect(screen.getByTestId("topbar-metrics")).toBeTruthy();
    expect(screen.getByTestId("topbar-metrics").className).toContain("h-8");
    expect(screen.getByLabelText("CPU 42%")).toBeTruthy();
    expect(subscribeMock).toHaveBeenCalledWith(true);
  });

  it("yields metrics ownership to the app status bar when enabled", () => {
    renderMetrics(true);

    expect(screen.queryByTestId("topbar-metrics")).toBeNull();
    expect(subscribeMock).not.toHaveBeenCalled();
  });

  it("uses 16px metric icons in the mobile topbar", () => {
    renderMetrics(false, true);

    expect(screen.getByLabelText("CPU 42%").querySelector("svg")?.getAttribute("class")).toContain(
      "size-4",
    );
  });
});
