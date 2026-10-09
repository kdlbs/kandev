import path from "node:path";
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

  // @covers AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.5
  test("shows the resolved backup directory from database stats", async ({
    testPage,
    apiClient,
  }) => {
    const response = await apiClient.rawRequest("GET", "/api/v1/system/database");
    const database = (await response.json()) as { path?: string; backup_directory?: string };
    expect(database.path).toBeTruthy();
    const backupDirectory = database.backup_directory;
    expect(backupDirectory).toBeTruthy();
    expect(path.isAbsolute(backupDirectory!)).toBe(true);
    expect(backupDirectory).toBe(path.resolve(path.dirname(database.path!), "backups"));

    await testPage.goto("/settings/system/data-storage?tab=database");
    await expect(
      testPage.getByText(`VACUUM INTO snapshots stored under ${backupDirectory}.`),
    ).toBeVisible();
  });

  test("explains each backup row action on hover", async ({ testPage }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/system/data-storage?tab=database");
    await testPage.getByTestId("system-backups-create").click();
    await expect(testPage.getByTestId("system-backups-table")).toBeVisible({ timeout: 15_000 });

    const row = testPage.locator('[data-testid="system-backups-row"]').first();
    const name = await row.getAttribute("data-name");
    expect(name).toBeTruthy();
    const tooltip = testPage.locator('[data-slot="tooltip-content"]:not([data-state="closed"])');
    for (const [testId, operation] of [
      ["system-backups-download", "Download"],
      ["system-backups-restore", "Restore"],
      ["system-backups-delete", "Delete"],
    ] as const) {
      const action = row.getByTestId(testId);
      await expect(action).toHaveAttribute("aria-label", `${operation} ${name}`);
      await action.hover();
      await expect(tooltip).toContainText(operation);
      await expect(tooltip).not.toContainText(name!);
    }
  });

  test("create a manual backup, see it in the table, then delete it back to the empty state", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/system/data-storage?tab=database");
    await expect(testPage.getByTestId("system-page-title")).toHaveText("Data & Logs");
    await expect(testPage.getByTestId("system-backups-card")).toBeVisible();

    // Empty state shows initially (no auto snapshots exist on this fresh boot path).
    await expect(testPage.getByTestId("system-backups-empty")).toBeVisible({ timeout: 10_000 });

    // Click create snapshot; the UI polls until the async VACUUM INTO job
    // has produced a new manual snapshot, then renders the table.
    await testPage.getByTestId("system-backups-create").click();
    await expect(testPage.getByTestId("system-backups-table")).toBeVisible({ timeout: 15_000 });

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

  test("revalidates an inactive backup list after returning from another settings route", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/system/data-storage?tab=database");
    await expect(testPage.getByTestId("system-backups-card")).toBeVisible();
    await expect
      .poll(
        async () =>
          (await testPage.getByTestId("system-backups-empty").isVisible()) ||
          (await testPage.getByTestId("system-backups-row").first().isVisible()),
        { timeout: 10_000 },
      )
      .toBe(true);

    await testPage.locator('a[href="/settings/system/status"]:visible').click();
    await expect(testPage).toHaveURL(/\/settings\/system\/status(?:\?|$)/);
    await expect(testPage.getByTestId("system-backups-card")).toHaveCount(0);

    const createResponse = await apiClient.rawRequest("POST", "/api/v1/system/backups");
    expect(createResponse.status).toBe(202);
    const { job_id: jobId } = (await createResponse.json()) as { job_id: string };
    let createdName: string | undefined;
    await expect
      .poll(
        async () => {
          const response = await apiClient.rawRequest("GET", `/api/v1/system/jobs/${jobId}`);
          expect(response.ok, response.statusText).toBe(true);
          const job = (await response.json()) as {
            state: string;
            result?: { name?: string };
          };
          createdName = job.result?.name;
          return job.state;
        },
        { timeout: 20_000, intervals: [250] },
      )
      .toBe("succeeded");
    expect(createdName).toMatch(/^manual-/);

    const revalidated = testPage.waitForResponse(
      (response) =>
        response.request().method() === "GET" &&
        new URL(response.url()).pathname.endsWith("/api/v1/system/backups"),
    );
    await testPage.locator('a[href^="/settings/system/data-storage"]:visible').click();
    await revalidated;
    await expect(testPage).toHaveURL(/\/settings\/system\/data-storage/);
    await expect(
      testPage.locator('[data-testid="system-backups-row"]').filter({ hasText: createdName! }),
    ).toBeVisible();
  });
});
