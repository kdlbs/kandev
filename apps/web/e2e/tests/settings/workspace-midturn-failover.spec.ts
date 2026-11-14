import { test, expect } from "../../fixtures/test-base";

type SavedWorkspace = { dynamic_midturn_failover_enabled?: boolean };

test.describe("Workspace mid-turn provider failover setting", () => {
  test("is hidden while dynamic routing is off", async ({ testPage, seedData }) => {
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}`);
    await expect(testPage.getByTestId("workspace-idle-policy-card")).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByTestId("workspace-dynamic-routing-card")).toHaveCount(0);
  });

  test("defaults off and saves the workspace opt-in", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "true",
    });
    const readSaved = async () => {
      const response = await apiClient.rawRequest(
        "GET",
        `/api/v1/workspaces/${seedData.workspaceId}`,
      );
      expect(response.ok).toBe(true);
      return (await response.json()) as SavedWorkspace;
    };

    try {
      expect((await readSaved()).dynamic_midturn_failover_enabled).toBe(false);
      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}`);
      const toggle = testPage.getByTestId("workspace-midturn-failover-switch");
      await expect(toggle).toBeVisible({ timeout: 15_000 });
      await expect(toggle).toHaveAttribute("aria-checked", "false");

      await toggle.click();
      await expect(toggle).toHaveAttribute("aria-checked", "true");
      const floatingSave = testPage.getByTestId("settings-floating-save");
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(floatingSave).not.toBeVisible();
      expect((await readSaved()).dynamic_midturn_failover_enabled).toBe(true);

      await testPage.reload();
      await expect(testPage.getByTestId("workspace-midturn-failover-switch")).toHaveAttribute(
        "aria-checked",
        "true",
        { timeout: 15_000 },
      );
    } finally {
      await apiClient.rawRequest("PATCH", `/api/v1/workspaces/${seedData.workspaceId}`, {
        dynamic_midturn_failover_enabled: false,
      });
      await releaseFeature();
    }
  });
});
