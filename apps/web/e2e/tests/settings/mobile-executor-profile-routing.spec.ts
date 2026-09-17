import type { Locator } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

const INITIAL_IMAGE_TAG = "kandev/e2e:mobile-routing-initial";
const EDITED_IMAGE_TAG = "kandev/e2e:mobile-routing-edited";
const DOCKERFILE = "FROM busybox\nWORKDIR /workspace\n";

function canonicalProfilePath(profileId: string): string {
  return `/settings/executors/${encodeURIComponent(profileId)}`;
}

async function expectTouchTarget(locator: Locator, label: string) {
  const box = await locator.boundingBox();
  expect(box, `${label} must have geometry`).not.toBeNull();
  expect(box!.height, `${label} must be at least 44px tall`).toBeGreaterThanOrEqual(44);
}

test("phone profile navigation reaches the complete Docker editor", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const executor = await apiClient.createExecutor(
    "E2E mobile profile routing executor",
    "local_docker",
  );
  const profile = await apiClient.createExecutorProfile(executor.id, {
    name: "E2E mobile profile routing Docker",
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
    const profileCard = testPage.getByText(profile.name, { exact: true });
    await expect(profileCard).toBeVisible();
    await profileCard.tap();
    await expect(testPage).toHaveURL(canonicalProfilePath(profile.id));

    await expect(testPage.locator("#image-tag")).toBeVisible();
    await expect(testPage.getByText("Dockerfile", { exact: true })).toBeVisible();
    const buildButton = testPage.getByRole("button", { name: "Build Image" });
    await expect(buildButton).toBeVisible();
    await expectTouchTarget(buildButton, "Mobile Docker build button");
    const buildResponse = waitForHttp(testPage, "POST", /^\/api\/v1\/docker\/build$/, {
      predicate: (response) => response.ok(),
    });
    await buildButton.tap();
    await buildResponse;
    await expect(testPage.getByText("Success", { exact: true })).toBeVisible();

    await testPage.locator("#image-tag").fill(EDITED_IMAGE_TAG);
    const floatingSave = testPage.getByTestId("settings-floating-save");
    const saveButton = floatingSave.getByRole("button", { name: "Save changes" });
    await expectTouchTarget(saveButton, "Mobile profile save button");
    const saveResponse = waitForHttp(
      testPage,
      "PATCH",
      /^\/api\/v1\/executors\/[^/]+\/profiles\/[^/]+$/,
      { predicate: (response) => response.ok() },
    );
    await saveButton.tap();
    await saveResponse;
    await expect(testPage.getByText("Profile saved")).toBeVisible();
    await expect
      .poll(
        async () => (await apiClient.getExecutorProfile(executor.id, profile.id)).config?.image_tag,
      )
      .toBe(EDITED_IMAGE_TAG);

    await testPage.reload();
    await expect(testPage.locator("#image-tag")).toHaveValue(EDITED_IMAGE_TAG);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile canonical executor profile editor");

    const legacyBookmark =
      `/settings/executor/${encodeURIComponent(executor.id)}/profile/${encodeURIComponent(profile.id)}` +
      "?tab=advanced#docker";
    await testPage.goto(legacyBookmark);
    await expect(testPage).toHaveURL(`${canonicalProfilePath(profile.id)}?tab=advanced#docker`);
    await expect(testPage.locator("#image-tag")).toHaveValue(EDITED_IMAGE_TAG);
    await expect(testPage.getByRole("button", { name: "Build Image" })).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile legacy executor profile bookmark");
  } finally {
    await apiClient.deleteExecutor(executor.id).catch(() => {});
  }
});
