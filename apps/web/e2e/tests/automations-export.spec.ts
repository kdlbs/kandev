import path from "node:path";
import fs from "node:fs";
import { execFileSync } from "node:child_process";
import { test, expect } from "../fixtures/test-base";
import { AutomationsPage } from "../pages/automations-page";
import { injectLatency } from "../helpers/causal-waits";

// AC-37/38: AutomationsExportButton (components/automations/automations-export-button.tsx)
// fetches the workspace's zip export, hands it to the browser as a Blob via a
// temporary <a download> anchor, and never navigates the endpoint directly.
// That anchor click is a real download, so Playwright's "download" event is
// the correct signal — the same pattern file-tree-download.spec.ts uses.

const EXPORT_ROUTE = "**/automations/export/zip";

test.describe("Automations export", () => {
  test("empty workspace still offers an enabled control that downloads a valid archive", async ({
    testPage,
    seedData,
    backend,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();
    await expect(automations.emptyState).toBeVisible({ timeout: 10_000 });

    // AC-32/AC-37: no automations is a valid export, not an error — the
    // control stays enabled rather than being disabled for an empty workspace.
    await expect(automations.exportButton).toBeEnabled();

    const downloadPromise = testPage.waitForEvent("download");
    await automations.exportButton.click();
    const download = await downloadPromise;

    // The filename comes from the object URL's `download` attribute, not
    // Content-Disposition — AC-37 pins this because a raw navigation would
    // only expose the header form.
    expect(download.suggestedFilename()).toBe("kandev-automations.zip");

    const savedPath = path.join(backend.tmpDir, `export-empty-${Date.now()}.zip`);
    await download.saveAs(savedPath);

    // A zero-entry zip is still a valid archive: it is exactly the 22-byte
    // End Of Central Directory record, signature `PK\x05\x06`. `unzip -l`
    // exits non-zero and prints a "zipfile is empty" warning for this shape
    // (verified against the real `archive/zip` writer), so it is not a usable
    // validity signal here — check the archive's own magic bytes instead.
    const bytes = fs.readFileSync(savedPath);
    expect(bytes.length).toBe(22);
    expect(bytes.subarray(0, 4).toString("hex")).toBe("504b0506");
  });

  test("workspace with automations exports one entry per automation", async ({
    testPage,
    seedData,
    apiClient,
    backend,
  }) => {
    await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Export Me One",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Export Me Two",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });

    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();
    await expect(automations.table).toBeVisible({ timeout: 10_000 });

    const downloadPromise = testPage.waitForEvent("download");
    await automations.exportButton.click();
    const download = await downloadPromise;

    const savedPath = path.join(backend.tmpDir, `export-two-${Date.now()}.zip`);
    await download.saveAs(savedPath);
    const listing = execFileSync("unzip", ["-l", savedPath], { encoding: "utf8" });

    // One .kandev/automations/<slug>.yml entry per automation (AC-24/25).
    const entryLines = listing
      .split("\n")
      .filter((line) => line.includes(".kandev/automations/") && line.endsWith(".yml"));
    expect(entryLines).toHaveLength(2);
  });

  test("control disables for the whole request span and re-enables once the download hands off", async ({
    testPage,
    seedData,
    backend,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();
    await expect(automations.emptyState).toBeVisible({ timeout: 10_000 });

    // The delay is the stimulus that makes the in-flight disabled state
    // observable, not a synchronization wait — real latency injection per
    // e2e/helpers/causal-waits.ts.
    await testPage.route(EXPORT_ROUTE, async (route) => {
      await injectLatency(500, "make the export control's disabled-in-flight window observable");
      await route.continue();
    });

    const downloadPromise = testPage.waitForEvent("download");
    await automations.exportButton.click();
    await expect(automations.exportButton).toBeDisabled();

    const download = await downloadPromise;
    await download.saveAs(path.join(backend.tmpDir, `export-inflight-${Date.now()}.zip`));

    await expect(automations.exportButton).toBeEnabled();
  });

  test("surfaces a non-blocking error and leaves the list page unchanged on a non-200 response", async ({
    testPage,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();
    await expect(automations.emptyState).toBeVisible({ timeout: 10_000 });

    await testPage.route(EXPORT_ROUTE, async (route) => {
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "simulated export failure" }),
      });
    });

    await automations.exportButton.click();

    await expect(testPage.locator("[data-sonner-toast]")).toContainText("simulated export failure");

    // "Leaving the page otherwise unchanged" per AC-37: still on the listings
    // page, still showing the empty state, control re-enabled for a retry.
    await expect(automations.listPage).toBeVisible();
    await expect(automations.emptyState).toBeVisible();
    await expect(automations.exportButton).toBeEnabled();
  });
});
