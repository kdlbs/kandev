import { expect, type Page } from "@playwright/test";

export const DATABASE_SETTINGS_ROUTE = "/settings/system/data-storage?tab=database";

type DatabaseStatsFixture = {
  driver: string;
  path: string;
  backup_directory: string;
  size_bytes: number;
  wal_size_bytes: number;
  message_content_bytes: number | null;
  message_metadata_bytes: number | null;
  message_payload_bytes: number | null;
  git_snapshot_bytes: number | null;
  logical_stats_state: "pending" | "ready" | "refreshing" | "stale" | "unavailable";
  logical_stats_measured_at: string | null;
  logical_stats_error?: string;
  metadata_stale: boolean;
  metadata_measured_at: string | null;
  schema_version: string;
  last_backup_at: string | null;
};

function databaseStats(overrides: Partial<DatabaseStatsFixture> = {}): DatabaseStatsFixture {
  return {
    driver: "sqlite",
    path: "/tmp/kandev.db",
    backup_directory: "/tmp/backups",
    size_bytes: 4096,
    wal_size_bytes: 256,
    message_content_bytes: null,
    message_metadata_bytes: null,
    message_payload_bytes: null,
    git_snapshot_bytes: null,
    logical_stats_state: "pending",
    logical_stats_measured_at: null,
    metadata_stale: false,
    metadata_measured_at: "2026-09-27T10:00:00Z",
    schema_version: "1",
    last_backup_at: null,
    ...overrides,
  };
}

export async function routeDatabaseStatsSequence(page: Page) {
  let current: "pending" | "refreshing" | "stale" | "ready" = "pending";
  const responses = {
    pending: databaseStats(),
    refreshing: databaseStats({
      logical_stats_state: "refreshing",
      logical_stats_measured_at: "2026-09-27T09:45:00Z",
      message_content_bytes: 128,
      message_metadata_bytes: 64,
      message_payload_bytes: 256,
      git_snapshot_bytes: 512,
    }),
    stale: databaseStats({
      logical_stats_state: "stale",
      logical_stats_measured_at: "2026-09-27T09:45:00Z",
      logical_stats_error: "scan_failed",
      message_content_bytes: 128,
      message_metadata_bytes: 64,
      message_payload_bytes: 256,
      git_snapshot_bytes: 512,
      metadata_stale: true,
    }),
    ready: databaseStats({
      logical_stats_state: "ready",
      logical_stats_measured_at: "2026-09-27T10:05:00Z",
      message_content_bytes: 144,
      message_metadata_bytes: 72,
      message_payload_bytes: 288,
      git_snapshot_bytes: 576,
    }),
  };
  const pattern = "**/api/v1/system/database";
  await page.route(pattern, async (route) => {
    if (route.request().method() !== "GET") {
      await route.continue();
      return;
    }
    const response = responses[current as keyof typeof responses];
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(response),
    });
  });
  return {
    showRefreshing: () => {
      current = "refreshing";
    },
    showStale: () => {
      current = "stale";
    },
    recover: () => {
      current = "ready";
    },
    remove: async () => page.unroute(pattern),
  };
}

export async function expectDatabaseControls(page: Page) {
  await expect(page.getByTestId("system-db-path")).toHaveText("/tmp/kandev.db");
  await expect(page.getByText(/\/tmp\/backups/)).toBeVisible();
  await expect(page.getByTestId("system-vacuum-button")).toBeVisible();
  await expect(page.getByTestId("system-optimize-button")).toBeVisible();
  await expect(page.getByTestId("system-factory-reset-button")).toBeVisible();
}
