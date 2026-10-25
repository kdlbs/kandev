import { test, expect } from "../../fixtures/test-base";

test.describe("User namespace support on mobile", () => {
  test("enables and persists the Docker profile setting with a touch-sized control", async ({
    testPage,
    apiClient,
  }) => {
    const exec = await apiClient.createExecutor("e2e-userns-mobile", "local_docker");
    const profile = await apiClient.createExecutorProfile(exec.id, {
      name: "userns mobile",
      config: { image_tag: "kandev-agent:e2e" },
    });
    await testPage.route("**/api/v1/docker/containers?*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: '{"containers":[]}',
      });
    });

    try {
      await testPage.goto(`/settings/executors/${profile.id}`);
      const toggle = testPage.getByRole("switch", { name: "User namespace support" });
      const touchTarget = testPage.locator('label[for="allow-user-namespaces"]');
      await expect(toggle).not.toBeChecked();
      await expect(touchTarget).toBeVisible();

      const targetBox = await touchTarget.boundingBox();
      expect(targetBox).not.toBeNull();
      expect(targetBox!.height).toBeGreaterThanOrEqual(44);
      await touchTarget.tap();
      await expect(toggle).toBeChecked();

      await testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" })
        .tap();
      await expect(testPage.getByText("Profile saved")).toBeVisible();
      await expect
        .poll(async () => (await apiClient.getExecutorProfile(exec.id, profile.id)).config)
        .toMatchObject({ allow_user_namespaces: "true" });

      await testPage.reload();
      await expect(toggle).toBeChecked();
      const horizontalOverflow = await testPage.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      );
      expect(horizontalOverflow).toBe(0);
    } finally {
      await apiClient.deleteExecutor(exec.id).catch(() => {});
    }
  });
});
