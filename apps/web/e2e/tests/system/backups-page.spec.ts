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

    await testPage.goto("/settings/system/data-storage");
    await expect(
      testPage.getByText(`VACUUM INTO snapshots stored under ${backupDirectory}.`),
    ).toBeVisible();
  });

  test("explains each backup row action on hover", async ({ testPage }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/system/data-storage");
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

    await testPage.goto("/settings/system/data-storage");
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
});
