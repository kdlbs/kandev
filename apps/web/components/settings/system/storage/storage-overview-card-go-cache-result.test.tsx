import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StorageMaintenanceRun } from "@/lib/types/system";
import { StorageOverviewCard } from "./storage-overview-card";
import { degradedOverview } from "./storage-overview-card.test-fixtures";

afterEach(cleanup);

describe("StorageOverviewCard Go-cache result", () => {
  it("shows a partial latest result in the expanded cache row", () => {
    const run = {
      id: "go-cache-result",
      trigger: "manual",
      state: "failed",
      settings_snapshot: {
        ...degradedOverview.settings,
        go_cache: { ...degradedOverview.settings.go_cache, allow_cleanup_while_busy: true },
      },
      result: {
        go_cache: {
          result: { reclaimed_bytes: 2 * 1024 ** 3, bytes_after: null, partial: true },
        },
      },
      message: "partial",
      started_at: "2026-10-02T12:00:00Z",
    } satisfies StorageMaintenanceRun;

    render(
      <TooltipProvider>
        <StorageOverviewCard
          overview={degradedOverview}
          latestGoCacheRun={run}
          onRunGoCache={vi.fn()}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByTestId("storage-resource-go-cache-trigger"));
    const result = screen.getByTestId("storage-go-cache-inline-result");
    expect(result.textContent).toContain("2 GB removed");
    expect(result.textContent).toContain("The remaining cache size is unknown.");
    expect(result.textContent).toContain("Cleanup was partial.");
    expect(result.textContent).toContain(
      "This run allowed Go cache cleanup while tasks were active.",
    );
  });
});
