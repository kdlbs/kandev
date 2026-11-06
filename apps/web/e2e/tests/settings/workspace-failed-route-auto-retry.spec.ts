import { test, expect } from "../../fixtures/test-base";

type SavedWorkspace = {
  dynamic_midturn_failover_enabled?: boolean;
  dynamic_failed_route_auto_retry_enabled?: boolean;
  dynamic_failed_route_auto_retry_max?: number;
};

test.describe("Workspace automatic failed-route retry setting", () => {
  test("defaults off and saves the opt-in and its limit", async ({
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
      const initial = await readSaved();
      expect(initial.dynamic_failed_route_auto_retry_enabled).toBe(false);
      expect(initial.dynamic_failed_route_auto_retry_max).toBe(1);
      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}`);
      const toggle = testPage.getByTestId("workspace-failed-route-auto-retry-switch");
      const limit = testPage.getByTestId("workspace-failed-route-auto-retry-max");
      await expect(toggle).toBeVisible({ timeout: 15_000 });
      await expect(toggle).toHaveAttribute("aria-checked", "false");
      await expect(limit).toBeDisabled();

      await toggle.click();
      await expect(toggle).toHaveAttribute("aria-checked", "true");
      await limit.fill("4");
      await expect(
        testPage.getByTestId("workspace-failed-route-auto-retry-max-error"),
      ).toBeVisible();
      await limit.fill("2");
      const floatingSave = testPage.getByTestId("settings-floating-save");
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(floatingSave).not.toBeVisible();
      const saved = await readSaved();
      expect(saved.dynamic_failed_route_auto_retry_enabled).toBe(true);
      expect(saved.dynamic_failed_route_auto_retry_max).toBe(2);
      expect(saved.dynamic_midturn_failover_enabled).toBe(false);

      await testPage.reload();
      await expect(
        testPage.getByTestId("workspace-failed-route-auto-retry-switch"),
      ).toHaveAttribute("aria-checked", "true", { timeout: 15_000 });
      await expect(testPage.getByTestId("workspace-failed-route-auto-retry-max")).toHaveValue("2");
      await expect(testPage.getByTestId("workspace-midturn-failover-switch")).toHaveAttribute(
        "aria-checked",
        "false",
      );
    } finally {
      await apiClient.rawRequest("PATCH", `/api/v1/workspaces/${seedData.workspaceId}`, {
        dynamic_failed_route_auto_retry_enabled: false,
        dynamic_failed_route_auto_retry_max: 1,
      });
      await releaseFeature();
    }
  });
});
