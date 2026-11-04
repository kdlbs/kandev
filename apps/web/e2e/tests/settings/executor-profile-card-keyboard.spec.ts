import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { useRegularMode } from "../../helpers/regular-mode";
import {
  createFixtureExecutorProfile,
  installFixtureExecutorProvider,
} from "../../helpers/plugin-executor-profile";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";

useRegularMode();

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.28
for (const key of ["Enter", "Space"] as const) {
  test(`opens an existing provider profile with Tab and ${key}`, async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    let executorId = "";
    let profileId = "";
    try {
      const executor = await installFixtureExecutorProvider(testPage, apiClient);
      executorId = executor.id;
      await testPage.goto("/settings/executors");
      const profile = await createFixtureExecutorProfile(testPage, executor, false);
      profileId = profile.id;
      await testPage.goto("/settings/executors");
      const card = testPage.getByTestId(`executor-profiles-card-${executor.id}`);
      const row = card.getByTestId(`executor-profile-card-${profile.id}`);
      const add = card.getByRole("button", { name: "Add", exact: true });
      await expect(row).toBeVisible();
      await add.focus();
      await expect(add).toBeFocused();
      const deletes: string[] = [];
      testPage.on("request", (request) => {
        if (request.method() === "DELETE" && request.url().includes(`/profiles/${profile.id}`)) {
          deletes.push(request.url());
        }
      });
      await testPage.keyboard.press("Tab");
      const focus = await testPage.evaluate(() => ({
        tag: document.activeElement?.tagName,
        text: document.activeElement?.textContent?.trim(),
        label: document.activeElement?.getAttribute("aria-label"),
      }));
      expect(
        await row.evaluate((element) => element.contains(document.activeElement)),
        JSON.stringify(focus),
      ).toBe(true);
      expect(focus.text, JSON.stringify(focus)).toContain(profile.name);
      await testPage.keyboard.press(key);
      await expect(testPage).toHaveURL(`/settings/executors/${encodeURIComponent(profile.id)}`);
      await expect(testPage.getByTestId("plugin-executor-profile-page")).toBeVisible();
      expect(deletes).toEqual([]);
      expect(
        (
          await apiClient.rawRequest(
            "GET",
            `/api/v1/executors/${executor.id}/profiles/${profile.id}`,
          )
        ).ok,
      ).toBe(true);
    } finally {
      if (executorId && profileId) {
        await apiClient.rawRequest(
          "DELETE",
          `/api/v1/executors/${executorId}/profiles/${profileId}`,
        );
      }
      await uninstallFixturePlugin(apiClient);
    }
  });
}

test("pointer opens the selected profile and delete stays on the hub", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(120_000);
  let executorId = "";
  let selectedId = "";
  let siblingId = "";
  try {
    const executor = await installFixtureExecutorProvider(testPage, apiClient);
    executorId = executor.id;
    await testPage.goto("/settings/executors");
    const selected = await createFixtureExecutorProfile(testPage, executor, false);
    selectedId = selected.id;
    await testPage.goto("/settings/executors");
    const sibling = await createFixtureExecutorProfile(testPage, executor, false);
    siblingId = sibling.id;
    await testPage.goto("/settings/executors");
    const row = testPage.getByTestId(`executor-profile-card-${selected.id}`);
    await row.getByText(selected.name, { exact: true }).click();
    await expect(testPage).toHaveURL(`/settings/executors/${encodeURIComponent(selected.id)}`);
    await expect(testPage.getByTestId("plugin-executor-profile-page")).toBeVisible();
    await testPage.goto("/settings/executors");
    const deleted = waitForHttp(
      testPage,
      "DELETE",
      new RegExp(`/api/v1/executors/${executor.id}/profiles/${selected.id}$`),
      { predicate: (response) => response.ok() },
    );
    await row.getByTestId("executor-profile-delete-button").click();
    await deleted;
    await expect(row).toHaveCount(0);
    await expect(testPage).toHaveURL(/\/settings\/executors$/);
    expect(
      (
        await apiClient.rawRequest(
          "GET",
          `/api/v1/executors/${executor.id}/profiles/${selected.id}`,
        )
      ).ok,
    ).toBe(false);
    expect(
      (await apiClient.rawRequest("GET", `/api/v1/executors/${executor.id}/profiles/${sibling.id}`))
        .ok,
    ).toBe(true);
  } finally {
    if (executorId && selectedId) {
      await apiClient.rawRequest(
        "DELETE",
        `/api/v1/executors/${executorId}/profiles/${selectedId}`,
      );
    }
    if (executorId && siblingId) {
      await apiClient.rawRequest("DELETE", `/api/v1/executors/${executorId}/profiles/${siblingId}`);
    }
    await uninstallFixturePlugin(apiClient);
  }
});

test("old chevron and row padding open without deleting a profile", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(120_000);
  let executorId = "";
  let selectedId = "";
  let siblingId = "";
  try {
    const executor = await installFixtureExecutorProvider(testPage, apiClient);
    executorId = executor.id;
    await testPage.goto("/settings/executors");
    const selected = await createFixtureExecutorProfile(testPage, executor, false);
    selectedId = selected.id;
    await testPage.goto("/settings/executors");
    const sibling = await createFixtureExecutorProfile(testPage, executor, false);
    siblingId = sibling.id;
    await testPage.goto("/settings/executors");
    const row = testPage.getByTestId(`executor-profile-card-${selected.id}`);
    await row.scrollIntoViewIfNeeded();
    const box = await row.boundingBox();
    expect(box).not.toBeNull();
    if (!box) throw new Error("Profile row has no geometry");
    const targetPath = `/settings/executors/${encodeURIComponent(selected.id)}`;
    const outcome = Promise.race([
      testPage.waitForURL(targetPath).then(() => "opened"),
      testPage
        .waitForResponse(
          (response) =>
            response.request().method() === "DELETE" &&
            response.url().endsWith(`/profiles/${selected.id}`),
        )
        .then((response) => `deleted:${response.status()}`),
    ]);
    await testPage.mouse.click(box.x + box.width - 20, box.y + box.height / 2);
    expect(await outcome, "the old chevron position must open, not delete").toBe("opened");
    await expect(testPage.getByTestId("plugin-executor-profile-page")).toBeVisible();
    expect(
      (
        await apiClient.rawRequest(
          "GET",
          `/api/v1/executors/${executor.id}/profiles/${selected.id}`,
        )
      ).ok,
    ).toBe(true);
    expect(
      (await apiClient.rawRequest("GET", `/api/v1/executors/${executor.id}/profiles/${sibling.id}`))
        .ok,
    ).toBe(true);
    await testPage.goto("/settings/executors");
    await row.scrollIntoViewIfNeeded();
    const paddingBox = await row.boundingBox();
    if (!paddingBox) throw new Error("Profile row has no geometry");
    await testPage.mouse.click(paddingBox.x + 4, paddingBox.y + paddingBox.height / 2);
    await expect(testPage).toHaveURL(targetPath);
    expect(
      (
        await apiClient.rawRequest(
          "GET",
          `/api/v1/executors/${executor.id}/profiles/${selected.id}`,
        )
      ).ok,
    ).toBe(true);
  } finally {
    if (executorId && selectedId)
      await apiClient.rawRequest(
        "DELETE",
        `/api/v1/executors/${executorId}/profiles/${selectedId}`,
      );
    if (executorId && siblingId)
      await apiClient.rawRequest("DELETE", `/api/v1/executors/${executorId}/profiles/${siblingId}`);
    await uninstallFixturePlugin(apiClient);
  }
});
