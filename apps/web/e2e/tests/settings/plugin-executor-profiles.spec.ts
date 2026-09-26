import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { waitForHttp } from "../../helpers/causal-waits";
import { useRegularMode } from "../../helpers/regular-mode";
import {
  createFixtureExecutorProfile,
  FIXTURE_PROFILE_SECRET,
  installFixtureExecutorProvider,
  readFixtureProfile,
} from "../../helpers/plugin-executor-profile";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";

useRegularMode();

test.describe("remote executor profiles", () => {
  test("creates, edits, redacts secrets, and selects a provider in the task flow", async ({
    testPage,
    backend,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_REMOTE_EXECUTOR_PLUGINS: "true",
    });
    try {
      const executor = await installFixtureExecutorProvider(testPage, apiClient);
      await testPage.goto("/settings/executors");
      const profile = await createFixtureExecutorProfile(testPage, executor, false);
      const editor = testPage.getByTestId("plugin-executor-profile-page");
      await expect(editor).toBeVisible();
      await expect(testPage.getByRole("heading", { name: /Fixture Remote Sandbox/ })).toBeVisible();
      await expect(editor).toContainText("Maximum lifetime: 8 hours.");
      await expect(editor.getByText("Credential is configured.", { exact: true })).toBeVisible();
      await expect(testPage.getByLabel("Credential")).toHaveValue("");

      let publicProfile = await readFixtureProfile(apiClient, executor.id, profile.id);
      expect(JSON.stringify(publicProfile)).not.toContain(FIXTURE_PROFILE_SECRET);
      expect(publicProfile.secret_fields?.credential).toBe(true);

      await testPage.getByLabel("Image").fill("fixture-agent:v2");
      const saveButton = testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" });
      const imageSaved = waitForHttp(
        testPage,
        "PATCH",
        new RegExp(`/api/v1/executors/${executor.id}/profiles/${profile.id}$`),
        { predicate: (response) => response.ok() },
      );
      await saveButton.click();
      await imageSaved;
      await testPage.reload();
      await expect(testPage.getByLabel("Image")).toHaveValue("fixture-agent:v2");
      await expect(testPage.getByText("Credential is configured.", { exact: true })).toBeVisible();

      const replacementSecret = "replacement-secret-never-return-this";
      await testPage.getByLabel("Credential").fill(replacementSecret);
      const secretSaved = waitForHttp(
        testPage,
        "PATCH",
        new RegExp(`/api/v1/executors/${executor.id}/profiles/${profile.id}$`),
        { predicate: (response) => response.ok() },
      );
      await saveButton.click();
      await secretSaved;
      publicProfile = await readFixtureProfile(apiClient, executor.id, profile.id);
      expect(JSON.stringify(publicProfile)).not.toContain(replacementSecret);
      expect(publicProfile.secret_fields?.credential).toBe(true);

      await testPage.getByTestId("executor-profile-secret-clear-credential").click();
      const secretCleared = waitForHttp(
        testPage,
        "PATCH",
        new RegExp(`/api/v1/executors/${executor.id}/profiles/${profile.id}$`),
        { predicate: (response) => response.ok() },
      );
      await saveButton.click();
      await secretCleared;
      publicProfile = await readFixtureProfile(apiClient, executor.id, profile.id);
      expect(JSON.stringify(publicProfile)).not.toContain(replacementSecret);
      expect(publicProfile.secret_fields?.credential).not.toBe(true);

      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.click();
      const selector = testPage.getByTestId("executor-profile-selector");
      await expect(selector).toBeVisible();
      await selector.click();
      const option = testPage.getByRole("option", { name: profile.name });
      await expect(option).toContainText("Maximum lifetime: 8 hours.");
      await option.click();
      await expect(selector).toContainText(profile.name);
    } finally {
      await uninstallFixturePlugin(apiClient);
      await releaseFeature();
    }
  });
});
