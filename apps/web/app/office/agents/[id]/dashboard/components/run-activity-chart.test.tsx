import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { RunActivityChart } from "./run-activity-chart";

afterEach(() => cleanup());

describe("RunActivityChart", () => {
  it("renders every backend outcome bucket in each bar and the legend", () => {
    render(
      <RunActivityChart
        days={[
          {
            date: "2026-08-28",
            succeeded: 1,
            skipped: 2,
            unclassified: 3,
            failed: 4,
            other: 5,
            total: 15,
          },
        ]}
      />,
    );

    const bar = screen.getByTestId("stacked-bar");
    for (const key of ["succeeded", "skipped", "unclassified", "failed", "other"]) {
      expect(bar.querySelector(`[data-segment-key="${key}"]`)).not.toBeNull();
    }
    expect(screen.getByTestId("run-activity-legend").children).toHaveLength(5);
  });
});
