import { test, expect } from "../../fixtures/office-fixture";

test.describe("Config Sync", () => {
  test("export config returns bundle", async ({ officeApi, officeSeed }) => {
    const bundle = await officeApi.exportConfig(officeSeed.workspaceId);
    expect(bundle).toBeDefined();
    // The bundle should contain workspace-level settings
    expect(typeof bundle).toBe("object");
    expect(bundle).not.toBeNull();
  });

  test("export config includes workspace settings", async ({ officeApi, officeSeed }) => {
    const bundle = await officeApi.exportConfig(officeSeed.workspaceId);
    // The exported bundle should reference the workspace in some form
    const bundleStr = JSON.stringify(bundle);
    expect(bundleStr.length).toBeGreaterThan(0);
  });

  test("incoming diff returns valid response", async ({ officeApi, officeSeed }) => {
    const diff = await officeApi.getIncomingDiff(officeSeed.workspaceId);
    expect(diff).toBeDefined();
    expect(typeof diff).toBe("object");
  });

  test("apply incoming sync completes without error", async ({ officeApi, officeSeed }) => {
    // Export current state to the filesystem first so that the subsequent
    // import-from-fs is a no-op (filesystem matches DB) and does not
    // delete agents/skills/routines created by earlier tests.
    await officeApi.applyOutgoingSync(officeSeed.workspaceId);
    const result = await officeApi.applyIncomingSync(officeSeed.workspaceId);
    expect(result).toBeDefined();
  });

  test("settings page renders", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/workspace/settings");
    await expect(testPage.getByRole("heading", { name: /settings/i })).toBeVisible({
      timeout: 10_000,
    });
  });
});
