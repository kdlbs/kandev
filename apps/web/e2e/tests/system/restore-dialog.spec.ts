import { test, expect } from "../../fixtures/test-base";

async function listBackups(apiClient: { rawRequest: (m: string, p: string) => Promise<Response> }) {
  const res = await apiClient.rawRequest("GET", "/api/v1/system/backups");
  if (!res.ok) return [] as Array<{ name: string; kind: string }>;
  const body = (await res.json()) as { snapshots?: Array<{ name: string; kind: string }> };
  return body.snapshots ?? [];
}

async function deleteAllManual(apiClient: {
  rawRequest: (m: string, p: string) => Promise<Response>;
}) {
  const snapshots = await listBackups(apiClient);
  for (const snap of snapshots) {
    if (snap.kind === "manual") {
      await apiClient
        .rawRequest("DELETE", `/api/v1/system/backups/${encodeURIComponent(snap.name)}`)
        .catch(() => undefined);
    }
  }
}

test.describe("Restore snapshot dialog", () => {
  test.beforeEach(async ({ apiClient }) => {
    await deleteAllManual(apiClient);
  });

  test.afterEach(async ({ apiClient }) => {
    await deleteAllManual(apiClient);
  });

  test("confirm enables only on 'RESTORE'; cancel closes the modal", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    // Seed a manual backup via the API so the row exists when the page loads.
    const create = await apiClient.rawRequest("POST", "/api/v1/system/backups");
    expect(create.ok).toBeTruthy();

    // Poll the backups list until the new snapshot appears (creation is async).
    let found = false;
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
      const snaps = await listBackups(apiClient);
      if (snaps.some((s) => s.kind === "manual")) {
        found = true;
        break;
      }
      await new Promise((r) => setTimeout(r, 250));
    }
    expect(found).toBeTruthy();

    await testPage.goto("/settings/system/backups");
    await expect(testPage.getByTestId("system-backups-table")).toBeVisible({ timeout: 15_000 });

    const firstRow = testPage.locator('[data-testid="system-backups-row"]').first();
    await firstRow.getByTestId("system-backups-restore").click();

    const dialog = testPage.getByTestId("system-restore-dialog");
    await expect(dialog).toBeVisible();

    const confirm = testPage.getByTestId("system-restore-confirm");
    const input = testPage.getByTestId("system-restore-input");

    await expect(confirm).toBeDisabled();
    await input.fill("WRONG");
    await expect(confirm).toBeDisabled();
    await input.fill("RESTORE");
    await expect(confirm).toBeEnabled();

    // DO NOT click confirm — would actually restart the backend.
    await testPage.getByTestId("system-restore-cancel").click();
    await expect(dialog).not.toBeVisible();
  });
});
