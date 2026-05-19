import { test, expect } from "../../fixtures/test-base";

async function deleteAllManualBackups(apiClient: {
  rawRequest: (m: string, p: string) => Promise<Response>;
}) {
  const res = await apiClient.rawRequest("GET", "/api/v1/system/backups");
  if (!res.ok) return;
  const body = (await res.json()) as { snapshots?: Array<{ name: string; kind: string }> };
  for (const snap of body.snapshots ?? []) {
    if (snap.kind === "manual") {
      await apiClient
        .rawRequest("DELETE", `/api/v1/system/backups/${encodeURIComponent(snap.name)}`)
        .catch(() => undefined);
    }
  }
}

test.describe("System Backups page", () => {
  test.beforeEach(async ({ apiClient }) => {
    await deleteAllManualBackups(apiClient);
  });

  test.afterEach(async ({ apiClient }) => {
    await deleteAllManualBackups(apiClient);
  });

  test("create a manual backup, see it in the table, then delete it back to the empty state", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/system/backups");
    await expect(testPage.getByTestId("system-page-title")).toHaveText("Backups");
    await expect(testPage.getByTestId("system-backups-card")).toBeVisible();

    // Empty state shows initially (no auto snapshots exist on this fresh boot path).
    await expect(testPage.getByTestId("system-backups-empty")).toBeVisible({ timeout: 10_000 });

    // Click create snapshot. The backend runs VACUUM INTO asynchronously and
    // reports progress via system.job.update WS events. Wait for the job
    // progress indicator to reach "succeeded" before expecting the table,
    // which is populated by the reload that fires on job success.
    await testPage.getByTestId("system-backups-create").click();
    await expect(
      testPage.locator('[data-testid="system-job-backup-create"][data-state="succeeded"]'),
    ).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("system-backups-table")).toBeVisible({ timeout: 10_000 });

    const rows = testPage.locator('[data-testid="system-backups-row"]');
    await expect(rows.first()).toBeVisible();

    // The newly created row has a manual- prefix and the kind badge says "manual".
    const firstName = await rows.first().getAttribute("data-name");
    expect(firstName ?? "").toMatch(/^manual-/);
    await expect(rows.first()).toContainText("manual");

    // Delete the new row → empty state returns.
    await rows.first().getByTestId("system-backups-delete").click();
    await expect(testPage.getByTestId("system-backups-empty")).toBeVisible({ timeout: 10_000 });
  });
});
