import fs from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";

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
      summary: Record<string, unknown>;
    };
    body.capabilities.temporary_artifacts_available = true;
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
