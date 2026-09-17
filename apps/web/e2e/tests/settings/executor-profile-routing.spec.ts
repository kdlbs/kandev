import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { setSettingsMenuMode } from "../../helpers/settings-menu";

const INITIAL_IMAGE_TAG = "kandev/e2e:routing-initial";
const EDITED_IMAGE_TAG = "kandev/e2e:routing-edited";
const DRAFT_IMAGE_TAG = "kandev/e2e:routing-draft";
const DOCKERFILE = "FROM busybox\nWORKDIR /workspace\n";

function canonicalProfilePath(profileId: string): string {
  return `/settings/executors/${encodeURIComponent(profileId)}`;
}

async function expectDockerProfileEditor(testPage: Page, profileId: string) {
  await expect(testPage).toHaveURL(canonicalProfilePath(profileId));
  await expect(testPage.locator("#image-tag")).toBeVisible();
  await expect(testPage.getByText("Dockerfile", { exact: true })).toBeVisible();
  await expect(testPage.getByRole("button", { name: "Build Image" })).toBeVisible();
}

test.describe("executor profile routing", () => {
  test("converges desktop entry points on the complete Docker profile editor", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    const executor = await apiClient.createExecutor("E2E profile routing executor", "local_docker");
    const profile = await apiClient.createExecutorProfile(executor.id, {
      name: "E2E profile routing Docker",
      config: { dockerfile: DOCKERFILE, image_tag: INITIAL_IMAGE_TAG },
      prepare_script: "",
      cleanup_script: "",
      env_vars: [],
    });

    await testPage.route("**/api/v1/docker/build", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/x-ndjson",
        body: JSON.stringify({ stream: "Successfully built\n" }) + "\n",
      });
    });

    try {
      await testPage.goto("/settings/executors");
      await testPage.getByText(profile.name, { exact: true }).click();
      await expectDockerProfileEditor(testPage, profile.id);

      await testPage.getByRole("button", { name: "Build Image" }).click();
      await expect(testPage.getByText("Success", { exact: true })).toBeVisible({
        timeout: 10_000,
      });

      await testPage.locator("#image-tag").fill(EDITED_IMAGE_TAG);
      const floatingSave = testPage.getByTestId("settings-floating-save");
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(testPage.getByText("Profile saved")).toBeVisible({ timeout: 10_000 });
      await expect
        .poll(
          async () =>
            (await apiClient.getExecutorProfile(executor.id, profile.id)).config?.image_tag,
        )
        .toBe(EDITED_IMAGE_TAG);

      await testPage.reload();
      await expect(testPage.locator("#image-tag")).toHaveValue(EDITED_IMAGE_TAG);

      await setSettingsMenuMode(testPage, "accordion");
      const settingsTree = testPage.getByTestId("app-sidebar-settings-mode");
      const expandExecutors = settingsTree.getByRole("button", { name: /Expand Executors/i });
      if (await expandExecutors.count()) await expandExecutors.click();
      const expandExecutor = settingsTree.getByRole("button", {
        name: `Expand ${executor.name}`,
      });
      if (await expandExecutor.count()) await expandExecutor.click();
      const profileLink = settingsTree.locator(`a[href="${canonicalProfilePath(profile.id)}"]`);
      await expect(profileLink).toBeVisible({ timeout: 10_000 });
      await profileLink.click();
      await expectDockerProfileEditor(testPage, profile.id);

      await testPage.goto(`/settings/executor/${encodeURIComponent(executor.id)}`);
      await testPage.getByText(profile.name, { exact: true }).click();
      await expectDockerProfileEditor(testPage, profile.id);

      const legacyBookmark =
        `/settings/executor/${encodeURIComponent(executor.id)}/profile/${encodeURIComponent(profile.id)}` +
        "?tab=advanced#docker";
      await testPage.goto(legacyBookmark);
      await expect(testPage).toHaveURL(`${canonicalProfilePath(profile.id)}?tab=advanced#docker`);
      await expect(testPage.locator("#image-tag")).toHaveValue(EDITED_IMAGE_TAG);
      await expect(testPage.getByRole("button", { name: "Build Image" })).toBeVisible();

      await testPage.locator("#image-tag").fill(DRAFT_IMAGE_TAG);
      const executorsLink = settingsTree.locator('a[href="/settings/executors"]');
      await executorsLink.click();
      const leaveDialog = testPage.getByRole("alertdialog");
      await expect(leaveDialog).toBeVisible();
      await leaveDialog.getByRole("button", { name: "Continue editing" }).click();
      await expect(testPage).toHaveURL(`${canonicalProfilePath(profile.id)}?tab=advanced#docker`);
      await expect(testPage.locator("#image-tag")).toHaveValue(DRAFT_IMAGE_TAG);

      await executorsLink.click();
      await expect(leaveDialog).toBeVisible();
      await leaveDialog.getByRole("button", { name: "Discard and leave" }).click();
      await expect(testPage).toHaveURL(/\/settings\/executors$/);

      await testPage.goto(canonicalProfilePath(profile.id));
      await expect(testPage.locator("#image-tag")).toHaveValue(EDITED_IMAGE_TAG);
    } finally {
      await apiClient.deleteExecutor(executor.id).catch(() => {});
    }
  });
});
