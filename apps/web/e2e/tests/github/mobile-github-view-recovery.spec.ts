import { test, expect } from "../../fixtures/test-base";
import { MobileGitHubPage } from "../../pages/mobile-github-page";

// @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.1
test("saved-query load failure is recoverable, not an empty collection", async ({
  testPage,
  apiClient,
}) => {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  let unavailable = true;
  await testPage.route("**/api/v1/github/workspace-settings?**", async (route) => {
    if (unavailable) await route.fulfill({ status: 503, json: { error: "unavailable" } });
    else await route.continue();
  });
  const github = new MobileGitHubPage(testPage);
  await github.goto();
  await github.mobileMenuButton.tap();
  await expect(github.mobileSidebar.getByRole("alert")).toContainText(
    "Saved queries could not be loaded",
  );
  await expect(github.mobileSidebar.getByText("No saved queries yet.")).toBeHidden();
  await expect(
    github.mobileSidebar.getByRole("button", { name: "Save current query" }),
  ).toBeDisabled();
  unavailable = false;
  await github.mobileSidebar.getByRole("button", { name: "Retry", exact: true }).tap();
  await expect(github.mobileSidebar.getByRole("alert")).toBeHidden();
  await expect(
    github.mobileSidebar.getByRole("button", { name: "Save current query" }),
  ).toBeEnabled();
  await testPage.unroute("**/api/v1/github/workspace-settings?**");
});

// @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.2
// @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.5
test("long Views collections scroll inside the drawer while header and Save stay reachable", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  const baseline = await testPage.request.get("/api/v1/github/workspace-settings", {
    params: { workspace_id: seedData.workspaceId },
  });
  expect(baseline.ok()).toBe(true);
  const settings = await baseline.json();
  const presets = Array.from({ length: 20 }, (_, i) => ({
    id: `long-view-${i}`,
    kind: "pr",
    label: `Team review queue ${i} with a long descriptive saved name`,
    customQuery: "is:open",
    repoFilter: "",
    createdAt: "2026-09-11T00:00:00Z",
  }));
  const seeded = await testPage.request.put("/api/v1/github/workspace-settings", {
    data: { workspace_id: seedData.workspaceId, saved_presets: presets },
  });
  expect(seeded.ok()).toBe(true);
  try {
    await testPage.setViewportSize({ width: 320, height: 640 });
    const github = new MobileGitHubPage(testPage);
    await github.goto();
    await expect(github.mobileMenuButton).toHaveAccessibleName(/Views: Review requested/);
    expect((await github.mobileMenuButton.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await github.mobileMenuButton.tap();
    await expect
      .poll(async () => {
        const bounds = await github.mobileSidebar.boundingBox();
        return Math.round((bounds?.y ?? 0) + (bounds?.height ?? 0));
      })
      .toBe(640);
    const scroller = github.mobileSidebar.getByTestId("github-views-scroll");
    await expect
      .poll(() => scroller.evaluate((element) => element.scrollHeight > element.clientHeight))
      .toBe(true);
    const save = github.mobileSidebar.getByRole("button", { name: "Save current query" });
    const before = await save.boundingBox();
    await scroller.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect(github.savedQueryByLabel(presets[19].label)).toBeInViewport();
    expect((await save.boundingBox())!.y).toBeCloseTo(before!.y, 0);
    await expect(save).toBeInViewport();
    await expect(
      github.mobileSidebar.getByRole("button", { name: "Done", exact: true }),
    ).toBeInViewport();
    expect(
      await github.mobileSidebar.evaluate((element) => element.scrollWidth <= element.clientWidth),
    ).toBe(true);
    await testPage.keyboard.press("Escape");
    await expect(github.mobileSidebar).toBeHidden();
    await expect(github.mobileMenuButton).toBeFocused();
  } finally {
    const restored = await testPage.request.put("/api/v1/github/workspace-settings", {
      data: { workspace_id: seedData.workspaceId, saved_presets: settings.saved_presets ?? [] },
    });
    expect(restored.ok()).toBe(true);
  }
});
