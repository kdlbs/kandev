import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { DatabaseStats } from "@/lib/types/system";
import { DatabaseStatsCard } from "./database-stats-card";

const mocks = vi.hoisted(() => ({
  value: {
    database: null as DatabaseStats | null,
    isLoading: false,
    error: null as string | null,
    reload: vi.fn(),
    retry: vi.fn(),
  },
}));

vi.mock("@/hooks/domains/system/use-database-stats", () => ({
  useDatabaseStats: () => mocks.value,
}));
vi.mock("./job-progress-indicator", () => ({ JobProgressIndicator: () => null }));
vi.mock("./factory-reset-dialog", () => ({ FactoryResetDialog: () => null }));

const database: DatabaseStats = {
  driver: "sqlite",
  path: "/data/kandev.db",
  backup_directory: "/data/backups",
  size_bytes: 1024,
  wal_size_bytes: 128,
  message_content_bytes: null,
  message_metadata_bytes: null,
  message_payload_bytes: null,
  git_snapshot_bytes: null,
  logical_stats_state: "unavailable",
  logical_stats_measured_at: null,
  logical_stats_error: "scan_failed",
  metadata_stale: true,
  metadata_measured_at: "2026-09-27T10:00:00Z",
  schema_version: "1",
  last_backup_at: null,
};

type DatabaseStatsResult = {
  database: DatabaseStats | null;
  isLoading: boolean;
  error: string | null;
  reload: () => Promise<void>;
  retry: () => Promise<void>;
};

beforeEach(() => {
  vi.resetAllMocks();
  mocks.value = { database: null, isLoading: false, error: null, reload: vi.fn(), retry: vi.fn() };
});
afterEach(cleanup);

function renderCard(stats: DatabaseStatsResult) {
  return render(
    <TooltipProvider>
      <DatabaseStatsCard stats={stats} />
    </TooltipProvider>,
  );
}

// @covers AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.5
it("keeps database details and maintenance actions visible when logical totals are unavailable", () => {
  renderCard({ ...mocks.value, database });

  expect(screen.getByTestId("system-db-path").textContent).toBe("/data/kandev.db");
  expect(screen.getByTestId("system-db-logical-stats-status").textContent).toMatch(
    /Logical totals are unavailable/i,
  );
  expect(screen.getByTestId("system-db-metadata-stale").textContent).toMatch(
    /Database details may be out of date/i,
  );
  expect(screen.getByTestId("system-vacuum-button")).toBeTruthy();
  expect(screen.getByTestId("system-factory-reset-button")).toBeTruthy();
});

it("offers a retry when the logical snapshot is unavailable", () => {
  const retry = vi.fn().mockResolvedValue(undefined);
  const reload = vi.fn().mockResolvedValue(undefined);
  renderCard({ ...mocks.value, database, retry, reload });
  fireEvent.click(screen.getByTestId("system-db-logical-stats-retry"));
  expect(retry).toHaveBeenCalledOnce();
  expect(reload).not.toHaveBeenCalled();
});

it("shows the last measurement time while an expired snapshot refreshes", () => {
  const refreshing: DatabaseStats = {
    ...database,
    logical_stats_state: "refreshing",
    logical_stats_measured_at: "2026-09-27T10:00:00Z",
  };
  renderCard({ ...mocks.value, database: refreshing });
  expect(screen.getByTestId("system-db-logical-stats-status").textContent).toMatch(
    /Logical totals measured .* Updating/i,
  );
});
