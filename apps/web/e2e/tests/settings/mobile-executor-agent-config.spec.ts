import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import {
  expectNoHorizontalOverflow,
  expectTouchTarget,
  PORTABLE_CONFIG_BUNDLE_ID,
  portableConfigInfo,
  portableConfigSection,
  selectPortableConfigBundle,
} from "./executor-agent-config-helpers";

test.describe("portable agent configuration settings on mobile", () => {
  test("uses a bottom drawer and keeps bundle controls reachable", async ({
    apiClient,
    backend,
    testPage,
  }) => {
    const configDir = path.join(backend.tmpDir, ".mock-agent");
    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(path.join(configDir, "settings.json"), '{"source":"settings"}\n');
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
      const agentTrigger = testPage.getByRole("button", { name: /Mock Not Configured/ });
      await expect(agentTrigger).toBeVisible();
      await agentTrigger.tap();

      const section = portableConfigSection(testPage);
      await expect(section).toBeVisible();

      const info = portableConfigInfo(testPage);
      await expectTouchTarget(info);
      await info.tap();
      const drawer = testPage.getByRole("dialog");
      await expect(drawer).toBeVisible();
      await expect(drawer).toContainText("Warm resumes keep the existing environment.");
      await testPage.keyboard.press("Escape");
      await expect(drawer).toBeHidden();

      const row = section.getByTestId(`portable-config-bundle-${PORTABLE_CONFIG_BUNDLE_ID}`);
      await expectTouchTarget(row);
      await selectPortableConfigBundle(testPage);
      await expectNoHorizontalOverflow(testPage);
    } finally {
      await apiClient.deleteExecutor(executor.id).catch(() => {});
    }
  });
});
