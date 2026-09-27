import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { waitForHttp } from "../../helpers/causal-waits";
import { expectControlHeight } from "../../helpers/control-sizing";
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
      const profilesCard = testPage.getByTestId(`executor-profiles-card-${executor.id}`);
      await profilesCard.getByRole("button", { name: "Add", exact: true }).click();
      const createForm = testPage.getByTestId("plugin-executor-profile-create-form");
      await expect(createForm).toBeVisible();
      await waitForFiniteAnimations(testPage.getByRole("dialog"));
      const nameInput = testPage.locator("#plugin-executor-profile-name");
      const cancelCreate = testPage.getByRole("button", { name: "Cancel", exact: true });
      const createProfile = testPage.getByRole("button", { name: "Create Profile", exact: true });
      // @covers AC-UI-CONTROL-SIZING-001.1, AC-UI-CONTROL-SIZING-001.3, AC-UI-CONTROL-SIZING-001.6
      await expectControlHeight(nameInput, 28);
      await expectControlHeight(cancelCreate, 28);
      await expectControlHeight(createProfile, 28);
      await cancelCreate.click();
      await expect(createForm).toBeHidden();

      const profile = await createFixtureExecutorProfile(testPage, executor, false);
      const editor = testPage.getByTestId("plugin-executor-profile-page");
      await expect(editor).toBeVisible();
      await expect(testPage.getByRole("heading", { name: /Fixture Remote Sandbox/ })).toBeVisible();
      await expect(editor).toContainText("Maximum lifetime: 8 hours.");
      await expect(editor.getByText("Credential is configured.", { exact: true })).toBeVisible();
      await expect(testPage.getByLabel("Credential")).toHaveValue("");

      for (const control of [
        testPage.locator("#executor-profile-name"),
        testPage.getByLabel("Region"),
        testPage.getByLabel("Image"),
        testPage.getByTestId("executor-profile-secret-clear-credential"),
        testPage.getByRole("button", { name: "Delete Profile", exact: true }),
        testPage.getByRole("button", { name: "Back to Executors", exact: true }),
      ]) {
        const box = await control.boundingBox();
        expect(box, "fine-pointer desktop profile controls must have geometry").not.toBeNull();
        expect(box!.height).toBeCloseTo(28, 0);
      }

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
