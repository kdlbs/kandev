import fs from "node:fs";
import path from "node:path";
import type { Page, Route } from "@playwright/test";

const ABOVE_DEFAULT_LIMIT_BYTES = 16 * 1024 * 1024 * 1024;

export function seedManagedGoCache(tmpDir: string): { artifact: string } {
  const cacheRoot = path.join(tmpDir, ".kandev", "cache");
  const cachePath = path.join(cacheRoot, "go-build");
  const artifact = path.join(cachePath, "e2e-sparse-artifact");
  fs.mkdirSync(cachePath, { recursive: true });
  fs.writeFileSync(path.join(cachePath, ".go-build.kandev-owned"), "kandev-managed-go-cache\n");
  fs.closeSync(fs.openSync(artifact, "w"));
  fs.truncateSync(artifact, ABOVE_DEFAULT_LIMIT_BYTES);
  return { artifact };
}

export async function mockTemporaryArtifactOverview(page: Page): Promise<void> {
  await page.route("**/api/v1/system/storage", async (route) => {
    if (route.request().method() !== "GET") {
      await route.continue();
      return;
    }
    // Fetch outside Playwright's route response lifecycle. The managed E2E
    // runner can dispose a route.fetch() response while a page navigation is
    // still settling, which makes response.body() intermittently unavailable.
    const requestHeaders = route.request().headers();
    const response = await fetch(route.request().url(), {
      headers: {
        ...(requestHeaders.accept ? { accept: requestHeaders.accept } : {}),
        ...(requestHeaders.cookie ? { cookie: requestHeaders.cookie } : {}),
      },
    });
    const body = JSON.parse(await response.text()) as {
      capabilities: Record<string, unknown>;
      summary: Record<string, unknown> | null;
    };
    body.capabilities.temporary_artifacts_available = true;
    body.summary ??= {};
    body.summary.temporary_artifacts = {
      available: true,
      total_count: 3,
      total_bytes: 48 * 1024 * 1024,
      active_count: 1,
      active_bytes: 16 * 1024 * 1024,
      protected_count: 1,
      protected_bytes: 16 * 1024 * 1024,
      stale_count: 1,
      stale_bytes: 16 * 1024 * 1024,
      skipped_count: 0,
    };
    await route.fulfill({
      status: response.status,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
}

type DatabaseFootprintStatus = "measured" | "unavailable" | "not_applicable";

type DatabaseStorageBody = {
  analysis?: Record<string, unknown> | null;
  analyzed_at?: string | null;
  summary?: Record<string, unknown> | null;
};

async function readDatabaseStorageRoute(route: Route): Promise<{
  response: Response;
  body: DatabaseStorageBody;
} | null> {
  if (route.request().method() !== "GET") {
    await route.continue();
    return null;
  }
  const requestHeaders = route.request().headers();
  const response = await fetch(route.request().url(), {
    headers: {
      ...(requestHeaders.accept ? { accept: requestHeaders.accept } : {}),
      ...(requestHeaders.cookie ? { cookie: requestHeaders.cookie } : {}),
    },
  });
  return {
    response,
    body: JSON.parse(await response.text()) as DatabaseStorageBody,
  };
}

function databaseMeasurement(
  status: DatabaseFootprintStatus,
  sizeBytes: number,
  resourcePath: string,
): Record<string, unknown> {
  if (status === "measured") {
    return {
      status,
      size_bytes: sizeBytes,
      path: resourcePath,
      included_in_total: true,
    };
  }
  return {
    status,
    included_in_total: false,
    reason: status === "unavailable" ? "measurement_failed" : "unsupported_driver",
  };
}

function updateDatabaseStorageBody(
  body: DatabaseStorageBody,
  options: {
    databasePath?: string;
    backupPath?: string;
    databaseBytes?: number;
    backupBytes?: number;
    databaseStatus: DatabaseFootprintStatus;
    backupStatus: DatabaseFootprintStatus;
  },
): void {
  const databaseBytes = options.databaseBytes ?? 8 * 1024 ** 3;
  const backupBytes = options.backupBytes ?? 3 * 1024 ** 3;
  const analyzedAt = new Date().toISOString();
  const summary = body.summary ?? {};
  body.summary = {
    ...summary,
    database: databaseMeasurement(
      options.databaseStatus,
      databaseBytes,
      options.databasePath ?? "/var/lib/kandev/kandev.db",
    ),
    database_backups: databaseMeasurement(
      options.backupStatus,
      backupBytes,
      options.backupPath ?? "/var/lib/kandev/backups",
    ),
  };
  const analysis = body.analysis ?? {};
  const currentProgress = (analysis.progress ?? {}) as Record<string, unknown>;
  const currentSources = (currentProgress.sources ?? {}) as Record<string, unknown>;
  body.analysis = {
    ...analysis,
    state: "ready",
    completed_at: analyzedAt,
    duration_ms: 100,
    cache_ttl_seconds: 900,
    refresh_due_at: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
    stale: false,
    error: null,
    progress: {
      ...currentProgress,
      completed_sources: 7,
      total_sources: 7,
      sources: {
        ...currentSources,
        database: {
          state: "ready",
          completed_items: 1,
          bytes_scanned: databaseBytes,
        },
        database_backups: {
          state: "ready",
          completed_items: 1,
          bytes_scanned: backupBytes,
        },
      },
    },
    partial_summary: null,
  };
  body.analyzed_at = analyzedAt;
}

export async function mockDatabaseFootprintOverview(
  page: Page,
  options: {
    databasePath?: string;
    backupPath?: string;
    databaseBytes?: number;
    backupBytes?: number;
    databaseStatus?: DatabaseFootprintStatus;
    backupStatus?: DatabaseFootprintStatus;
  } = {},
): Promise<void> {
  await page.route("**/api/v1/system/storage", async (route) => {
    const routed = await readDatabaseStorageRoute(route);
    if (!routed) return;
    const { response, body } = routed;
    const databaseStatus = options.databaseStatus ?? "measured";
    const backupStatus = options.backupStatus ?? "measured";
    updateDatabaseStorageBody(body, {
      ...options,
      databaseStatus,
      backupStatus,
    });
    await route.fulfill({
      status: response.status,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
}

export async function mockProgressiveStorageOverview(page: Page): Promise<{
  complete: () => void;
}> {
  let completed = false;
  await page.route("**/api/v1/system/storage", async (route) => {
    if (route.request().method() !== "GET") {
      await route.continue();
      return;
    }
    const requestHeaders = route.request().headers();
    const response = await fetch(route.request().url(), {
      headers: {
        ...(requestHeaders.accept ? { accept: requestHeaders.accept } : {}),
        ...(requestHeaders.cookie ? { cookie: requestHeaders.cookie } : {}),
      },
    });
    const body = JSON.parse(await response.text()) as {
      analysis?: unknown;
      analyzed_at?: string | null;
      capabilities?: Record<string, unknown>;
      summary?: Record<string, unknown> | null;
    };
    const startedAt = new Date(Date.now() - 1200).toISOString();
    const completedAt = new Date(Date.now()).toISOString();
    const refreshDueAt = new Date(Date.now() + 15 * 60 * 1000).toISOString();
    const sourceProgress = {
      workspaces: {
        state: "ready",
        completed_items: 3,
        total_items: 3,
        bytes_scanned: 2 * 1024 ** 3,
      },
      go_cache: {
        state: "ready",
        completed_items: 4,
        total_items: 4,
        bytes_scanned: 3 * 1024 ** 3,
      },
      quarantine: { state: "ready", completed_items: 1, total_items: 1, bytes_scanned: 1024 },
      temporary_artifacts: {
        state: "ready",
        completed_items: 1,
        total_items: 1,
        bytes_scanned: 1024,
      },
      docker: { state: "ready", completed_items: 1, total_items: 1, bytes_scanned: 1024 },
      database: {
        state: "ready",
        completed_items: 1,
        total_items: 1,
        bytes_scanned: 5 * 1024 ** 3,
      },
      database_backups: {
        state: "ready",
        completed_items: 1,
        total_items: 1,
        bytes_scanned: 3 * 1024 ** 3,
      },
    };
    body.analysis = completed
      ? {
          generation: 901,
          state: "ready",
          started_at: startedAt,
          completed_at: completedAt,
          duration_ms: 1200,
          cache_ttl_seconds: 900,
          refresh_due_at: refreshDueAt,
          stale: false,
          error: null,
          progress: { completed_sources: 7, total_sources: 7, sources: sourceProgress },
          partial_summary: null,
        }
      : {
          generation: 901,
          state: "scanning",
          started_at: startedAt,
          completed_at: null,
          duration_ms: null,
          cache_ttl_seconds: 900,
          refresh_due_at: null,
          stale: false,
          error: null,
          progress: {
            completed_sources: 1,
            total_sources: 7,
            sources: {
              ...sourceProgress,
              go_cache: {
                state: "scanning",
                completed_items: 1,
                total_items: 4,
                bytes_scanned: 10,
              },
              quarantine: { state: "pending", completed_items: 0, bytes_scanned: 0 },
              temporary_artifacts: { state: "pending", completed_items: 0, bytes_scanned: 0 },
              docker: { state: "pending", completed_items: 0, bytes_scanned: 0 },
              database: { state: "pending", completed_items: 0, bytes_scanned: 0 },
              database_backups: { state: "pending", completed_items: 0, bytes_scanned: 0 },
            },
          },
          partial_summary: {
            workspaces: {
              total_bytes: 2 * 1024 ** 3,
              active_bytes: 1 * 1024 ** 3,
              candidate_bytes: 1 * 1024 ** 3,
            },
          },
        };
    body.summary = completed
      ? {
          workspaces: {
            total_bytes: 4 * 1024 ** 3,
            active_bytes: 1 * 1024 ** 3,
            candidate_bytes: 2 * 1024 ** 3,
          },
          go_cache: {
            path: "/data/cache/go-build",
            size_bytes: 3 * 1024 ** 3,
            owned: true,
            enabled: false,
          },
          quarantine: { available: true, count: 1, size_bytes: 1024 },
          temporary_artifacts: {
            available: true,
            total_count: 1,
            total_bytes: 1024,
            active_count: 1,
            active_bytes: 1024,
            protected_count: 1,
            protected_bytes: 1024,
            stale_count: 0,
            stale_bytes: 0,
            skipped_count: 0,
          },
          docker: {
            available: false,
            build_cache_bytes: 0,
            image_layer_bytes: 0,
            unused_image_bytes: 0,
            managed_container_count: 0,
            managed_container_bytes: 0,
          },
          database: {
            status: "measured",
            size_bytes: 5 * 1024 ** 3,
            path: "/data/kandev.db",
            included_in_total: true,
          },
          database_backups: {
            status: "measured",
            size_bytes: 3 * 1024 ** 3,
            path: "/data/backups",
            included_in_total: true,
          },
        }
      : null;
    body.analyzed_at = completed ? completedAt : null;
    await route.fulfill({
      status: response.status,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
  return { complete: () => (completed = true) };
}
