import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import {
  UsageWindowRows,
  formatResetShort,
  shortWindowLabel,
  usageStatus,
} from "./usage-window-rows";
import type { ProviderUsage } from "@/lib/types/agent-profile";

afterEach(cleanup);

describe("shortWindowLabel", () => {
  it("shortens hour and day windows", () => {
    expect(shortWindowLabel("5-hour")).toBe("5h");
    expect(shortWindowLabel("7-day")).toBe("7d");
    expect(shortWindowLabel("30-day")).toBe("30d");
    expect(shortWindowLabel("7-day (Opus)")).toBe("7d (Opus)");
  });

  it("leaves unknown labels untouched", () => {
    expect(shortWindowLabel("primary")).toBe("primary");
  });
});

describe("formatResetShort", () => {
  const now = Date.UTC(2026, 6, 14, 12, 0, 0);

  it("formats minutes", () => {
    expect(formatResetShort(new Date(now + 42 * 60_000).toISOString(), now)).toBe("42m");
  });

  it("formats hours and minutes", () => {
    expect(formatResetShort(new Date(now + 3 * 3_600_000 + 17 * 60_000).toISOString(), now)).toBe(
      "3h 17m",
    );
  });

  it("formats days and hours", () => {
    expect(formatResetShort(new Date(now + 29 * 3_600_000).toISOString(), now)).toBe("1d 5h");
  });

  it("handles past reset times", () => {
    expect(formatResetShort(new Date(now - 60_000).toISOString(), now)).toBe("soon");
  });

  it("handles invalid timestamps", () => {
    expect(formatResetShort("not-a-date", now)).toBe("soon");
  });
});

function makeUsage(pcts: number[]): ProviderUsage {
  return {
    provider: "anthropic",
    plan: "max",
    windows: pcts.map((pct, i) => ({
      label: `${i + 1}-day`,
      utilization_pct: pct,
      reset_at: new Date(Date.now() + 3_600_000).toISOString(),
    })),
    fetched_at: new Date().toISOString(),
  };
}

describe("usageStatus", () => {
  it("is Good below 80%", () => {
    expect(usageStatus(makeUsage([10, 79])).label).toBe("Good");
  });

  it("is High at 80–89%", () => {
    expect(usageStatus(makeUsage([10, 85])).label).toBe("High");
  });

  it("is Critical at 90%+", () => {
    expect(usageStatus(makeUsage([10, 94])).label).toBe("Critical");
  });
});

describe("UsageWindowRows", () => {
  it("renders a colored bar, percentage, and reset time per window", () => {
    const usage: ProviderUsage = {
      provider: "anthropic",
      plan: "max",
      windows: [
        {
          label: "5-hour",
          utilization_pct: 94,
          reset_at: new Date(Date.now() + 2 * 3_600_000).toISOString(),
        },
        {
          label: "7-day",
          utilization_pct: 39,
          reset_at: new Date(Date.now() + 5 * 24 * 3_600_000).toISOString(),
        },
      ],
      fetched_at: new Date().toISOString(),
    };

    const { container, getByText } = render(<UsageWindowRows usage={usage} />);

    expect(getByText("5h")).toBeDefined();
    expect(getByText("94%")).toBeDefined();
    expect(getByText("7d")).toBeDefined();
    expect(getByText("39%")).toBeDefined();
    expect(getByText("4d 23h")).toBeDefined();
    expect(container.querySelector(".bg-red-500")).not.toBeNull();
    expect(container.querySelector(".bg-emerald-500")).not.toBeNull();
  });
});
