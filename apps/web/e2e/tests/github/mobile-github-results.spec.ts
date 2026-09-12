import { test, expect } from "../../fixtures/test-base";
import { MobileGitHubPage } from "../../pages/mobile-github-page";
import { waitForFiniteAnimations } from "../../helpers/animations";

// @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.3
// @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.4
// @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.5
test("phone results wrap long content, expose touch actions, and support every capped page", async ({
  testPage,
  apiClient,
}) => {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  await apiClient.mockGitHubAddPRs(
    Array.from({ length: 1050 }, (_, index) => ({
      number: index + 1,
      title: `Result ${index + 1}: A long pull request title that remains readable on a narrow phone screen`,
      state: "open",
      head_branch: `feature/${index}`,
      base_branch: "main",
      author_login: "test-user",
      repo_owner: "a-long-organization-name",
      repo_name: "a-very-long-repository-name-without-any-spaces",
    })),
  );
  // The mock provider returns every seeded PR; model GitHub's bounded page response.
  let resultTotal = 1050;
  await testPage.route("**/api/v1/github/user/prs?**", async (route) => {
    const response = await route.fetch();
    const data = await response.json();
    const params = new URL(route.request().url()).searchParams;
    const page = Number(params.get("page") ?? 1);
    const pageSize = Number(params.get("per_page") ?? 25);
    await route.fulfill({
      response,
      json: {
        ...data,
        total_count: resultTotal,
        prs: data.prs.slice(0, resultTotal).slice((page - 1) * pageSize, page * pageSize),
      },
    });
  });
  await testPage.setViewportSize({ width: 320, height: 640 });
  const github = new MobileGitHubPage(testPage);
  await github.goto();
  const query = testPage.getByPlaceholder(/Custom query/);
  await query.fill("is:open");
  await query.press("Enter");
  await expect(testPage.getByTestId("pr-row")).toHaveCount(25);
  const chooser = testPage.locator(
    'button[aria-label="Choose results page"],select[aria-label="Choose results page"]',
  );
  await expect(chooser).toBeVisible();
  expect(await chooser.evaluate((element) => element.tagName)).toBe("BUTTON");
  expect((await chooser.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await expect(testPage.getByRole("button", { name: "Previous page", exact: true })).toBeDisabled();
  const firstRow = testPage.getByTestId("pr-row").first();
  await expect(firstRow).toBeVisible();
  expect(await firstRow.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  const taskAction = firstRow.getByTestId("pr-start-task-trigger");
  expect((await taskAction.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await taskAction.tap();
  await expect(testPage.getByTestId("pr-start-task-preset").first()).toBeVisible();
  expect(
    (await testPage.getByTestId("pr-start-task-preset").first().boundingBox())!.height,
  ).toBeGreaterThanOrEqual(44);
  await testPage.keyboard.press("Escape");
  await chooser.tap();
  const pages = testPage.getByRole("dialog", { name: "Choose results page", exact: true });
  await expect(pages).toBeVisible();
  await waitForFiniteAnimations(pages);
  await expect(pages.getByRole("button", { name: /^Page \d+ of 40$/ })).toHaveCount(40);
  const pageFive = pages.getByRole("button", { name: "Page 5 of 40", exact: true });
  expect((await pageFive.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await pageFive.tap();
  await expect(pages).toBeHidden();
  await expect(chooser).toBeFocused();
  await expect(testPage.getByText("101–125 of 1000+", { exact: true })).toBeVisible();
  await chooser.tap();
  const lastPage = pages.getByRole("button", { name: "Page 40 of 40", exact: true });
  await lastPage.scrollIntoViewIfNeeded();
  await lastPage.tap();
  await expect(testPage.getByText("976–1000 of 1000+", { exact: true })).toBeVisible();
  await chooser.tap();
  await expect(lastPage).toBeInViewport();
  await expect(lastPage).toHaveAttribute("aria-current", "page");
  await testPage.keyboard.press("Escape");
  await expect(pages).toBeHidden();
  await expect(chooser).toBeFocused();
  await expect(testPage.getByRole("button", { name: "Next page", exact: true })).toBeDisabled();
  resultTotal = 50;
  await testPage.getByRole("button", { name: "Refresh", exact: true }).tap();
  await expect(chooser).toContainText("Page 40 of 2");
  await chooser.tap();
  await expect(pages.getByRole("button", { name: "Done", exact: true })).toBeFocused();
  await pages.getByRole("button", { name: "Page 2 of 2", exact: true }).tap();
  await expect(testPage.getByText("26–50 of 50", { exact: true })).toBeVisible();
  await expect(chooser).toBeFocused();
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
  await testPage.setViewportSize({ width: 767, height: 640 });
  await expect(chooser).toBeVisible();
  await testPage.setViewportSize({ width: 768, height: 640 });
  await expect(chooser).toBeHidden();
  await expect(github.inlineSidebar).toBeVisible();
});

// @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.2
// @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.5
test("long view names do not compress result counts and refresh metadata", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  await apiClient.mockGitHubAddPRs([
    {
      number: 1,
      title: "Harbor dashboard",
      state: "open",
      head_branch: "feature/mobile",
      base_branch: "main",
      author_login: "test-user",
      repo_owner: "harbor",
      repo_name: "web",
    },
  ]);
  const settings = await apiClient.rawRequest(
    "GET",
    `/api/v1/github/workspace-settings?workspace_id=${seedData.workspaceId}`,
  );
  const baseline = await settings.json();
  const saved = await apiClient.rawRequest("PUT", "/api/v1/github/workspace-settings", {
    workspace_id: seedData.workspaceId,
    saved_presets: [
      {
        id: "long-toolbar-view",
        kind: "pr",
        label: "Harbor navigation improvements ready for mobile review",
        customQuery: "is:open",
        repoFilter: "",
        createdAt: "2026-09-11T00:00:00Z",
        isDefault: true,
      },
    ],
  });
  expect(saved.ok).toBe(true);
  try {
    await testPage.setViewportSize({ width: 320, height: 640 });
    const github = new MobileGitHubPage(testPage);
    await github.goto();
    await expect(testPage.getByTestId("pr-row")).toBeVisible();
    const refresh = testPage.getByRole("button", { name: "Refresh", exact: true });
    const viewBounds = (await github.mobileMenuButton.boundingBox())!;
    const refreshBounds = (await refresh.boundingBox())!;
    expect(refreshBounds.y).toBeGreaterThanOrEqual(viewBounds.y + viewBounds.height);
    expect(viewBounds.height).toBeLessThanOrEqual(48);
    await expect(testPage.getByTestId("integration-mobile-result-count")).toHaveText("Result 1");
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await testPage.setViewportSize({ width: 767, height: 640 });
    await expect(testPage.getByTestId("integration-mobile-result-count")).toBeVisible();
    await testPage.setViewportSize({ width: 768, height: 640 });
    await expect(testPage.getByTestId("integration-mobile-result-count")).toBeHidden();
    await expect(github.inlineSidebar).toBeVisible();
  } finally {
    const restored = await apiClient.rawRequest("PUT", "/api/v1/github/workspace-settings", {
      workspace_id: seedData.workspaceId,
      saved_presets: baseline.saved_presets ?? [],
    });
    expect(restored.ok).toBe(true);
  }
});
