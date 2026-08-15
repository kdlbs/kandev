import { expect, test } from "../../fixtures/test-base";
import {
  expectNoHorizontalOverflow,
  expectTouchTarget,
  PORTABLE_CONFIG_BUNDLE_ID,
  portableConfigSection,
  selectPortableConfigBundle,
} from "./executor-agent-config-helpers";

test.describe("portable agent configuration settings on mobile", () => {
  test("uses a bottom drawer and keeps bundle controls reachable", async ({
    apiClient,
    testPage,
  }) => {
    const executor = await apiClient.createExecutor("E2E mobile portable config", "local_docker");
    const profile = await apiClient.createExecutorProfile(executor.id, {
      name: "E2E mobile portable config profile",
      config: {},
      prepare_script: "",
      cleanup_script: "",
      env_vars: [],
    });

    try {
      await testPage.goto(`/settings/executors/${profile.id}`);
      const section = portableConfigSection(testPage);
      await expect(section).toBeVisible();

      const info = testPage.getByTestId("portable-config-info");
      await expectTouchTarget(info);
      await info.tap();
      const drawer = testPage.getByRole("dialog");
      await expect(drawer).toBeVisible();
      await expect(drawer).toContainText("Warm resumes keep the existing environment.");
      await testPage.keyboard.press("Escape");
      await expect(drawer).toBeHidden();

      const row = testPage.getByTestId(`portable-config-bundle-${PORTABLE_CONFIG_BUNDLE_ID}`);
      await expectTouchTarget(row);
      await selectPortableConfigBundle(testPage);
      await expectNoHorizontalOverflow(testPage);
    } finally {
      await apiClient.deleteExecutor(executor.id).catch(() => {});
    }
  });
});
