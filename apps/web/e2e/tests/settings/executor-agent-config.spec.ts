import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import {
  MOCK_CODEX_CONFIG_BUNDLE_ID,
  portableConfigInfo,
  portableConfigSection,
  selectedBundleIds,
  selectPortableConfigBundle,
} from "./executor-agent-config-helpers";

test.describe("portable agent configuration settings", () => {
  test("saves configuration independently from authentication and reloads it", async ({
    apiClient,
    backend,
    testPage,
  }) => {
    // The baseline E2E agent has no credentials by design. Add the mock Codex
    // alias for this settings-only scenario so the auth checkbox remains a real,
    // independently selectable control without requiring provider secrets.
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "codex-acp" });
    const configDir = path.join(backend.tmpDir, ".mock-agent");
    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(path.join(configDir, "settings.json"), '{"source":"settings"}\n');
    const catalog = await apiClient.listAgentConfigBundles();
    expect(
      catalog.bundles.some(
        (bundle) => bundle.id === MOCK_CODEX_CONFIG_BUNDLE_ID && bundle.agent_id === "codex-acp",
      ),
    ).toBe(true);

    const executor = await apiClient.createExecutor("E2E portable config executor", "local_docker");
    const profile = await apiClient.createExecutorProfile(executor.id, {
      name: "E2E portable config profile",
      config: {},
      prepare_script: "",
      cleanup_script: "",
      env_vars: [],
    });

    try {
      await testPage.goto(`/settings/executors/${profile.id}`);

      const agentTrigger = testPage.getByRole("button", { name: /Mock Codex/ });
      await expect(agentTrigger).toBeVisible();
      await agentTrigger.click();

      const section = portableConfigSection(testPage, "codex-acp");
      await expect(section).toBeVisible();
      const info = portableConfigInfo(testPage, "codex-acp");
      await expect(info).toBeVisible();
      await info.hover();
      await expect(testPage.getByRole("tooltip")).toContainText("without changes");

      await selectPortableConfigBundle(testPage, MOCK_CODEX_CONFIG_BUNDLE_ID, "codex-acp");
      await expect(
        section
          .getByTestId(`portable-config-bundle-${MOCK_CODEX_CONFIG_BUNDLE_ID}`)
          .getByRole("checkbox"),
      ).toBeChecked();

      await testPage.getByRole("button", { name: "Mock Codex Not Configured" }).click();
      const authChoice = testPage.getByRole("checkbox", { name: "Copy auth files" }).first();
      await expect(authChoice).toBeVisible();
      await authChoice.click();
      await expect(authChoice).toBeChecked();

      const saveButton = testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" });
      await expect(saveButton).toBeEnabled();
      await saveButton.click();
      await expect(testPage.getByText("Profile saved")).toBeVisible();

      const saved = await apiClient.getExecutorProfile(executor.id, profile.id);
      expect(selectedBundleIds(saved.config)).toEqual([MOCK_CODEX_CONFIG_BUNDLE_ID]);
      expect(JSON.parse(saved.config?.remote_credentials ?? "[]").length).toBeGreaterThan(0);

      await testPage.reload();
      await testPage.getByRole("button", { name: /Mock Codex/ }).click();
      await expect(section).toBeVisible();
      await expect(
        section
          .getByTestId(`portable-config-bundle-${MOCK_CODEX_CONFIG_BUNDLE_ID}`)
          .getByRole("checkbox"),
      ).toBeChecked();
    } finally {
      await apiClient.deleteExecutor(executor.id).catch(() => {});
      await backend.restart();
    }
  });
});
