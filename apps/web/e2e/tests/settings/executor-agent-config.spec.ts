import { expect, test } from "../../fixtures/test-base";
import {
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
    // alias for this settings-only scenario so the auth radio remains a real,
    // independently selectable control without requiring provider secrets.
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "codex-acp" });
    const catalog = await apiClient.listAgentConfigBundles();
    expect(catalog.bundles.some((bundle) => bundle.id === "mock.settings")).toBe(true);

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

      const section = portableConfigSection(testPage);
      await expect(section).toBeVisible();
      await expect(testPage.getByTestId("portable-config-info")).toBeVisible();
      await testPage.getByTestId("portable-config-info").hover();
      await expect(testPage.getByRole("tooltip")).toContainText("without changes");

      await selectPortableConfigBundle(testPage);
      await expect(
        section.getByTestId("portable-config-bundle-mock.settings").getByRole("checkbox"),
      ).toBeChecked();

      await testPage.getByRole("button", { name: "Mock Codex Not Configured" }).click();
      const authChoice = testPage.getByRole("radio", { name: "Copy auth files" }).first();
      await expect(authChoice).toBeVisible();
      await authChoice.click();

      const saveButton = testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" });
      await expect(saveButton).toBeEnabled();
      await saveButton.click();
      await expect(testPage.getByText("Profile saved")).toBeVisible();

      const saved = await apiClient.getExecutorProfile(executor.id, profile.id);
      expect(selectedBundleIds(saved.config)).toEqual(["mock.settings"]);
      expect(JSON.parse(saved.config?.remote_credentials ?? "[]").length).toBeGreaterThan(0);

      await testPage.reload();
      await expect(section).toBeVisible();
      await expect(
        section.getByTestId("portable-config-bundle-mock.settings").getByRole("checkbox"),
      ).toBeChecked();
    } finally {
      await apiClient.deleteExecutor(executor.id).catch(() => {});
      await backend.restart();
    }
  });
});
