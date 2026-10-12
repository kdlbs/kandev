import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

// Below `md` there is no sidebar, so `/settings` is the settings list itself —
// the same tree the nav sheet shows, rendered as a real route rather than an
// overlay the app has to open for you.
test.describe("Settings index on a phone", () => {
  // @covers AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.9
  test("shows GitHub and GitLab badges only while their saved workspace toggles are on", async ({
    testPage,
    seedData,
    prCapture,
  }) => {
    for (const slug of ["github", "gitlab"]) {
      await testPage.route(`**/api/v1/${slug}/status?*`, (route) =>
        route.fulfill({
          json: {
            workspace_id: seedData.workspaceId,
            authenticated: true,
            token_configured: true,
            username: "badge-test",
            auth_method: "pat",
            host: slug === "gitlab" ? "gitlab.com" : "github.com",
            required_scopes: [],
          },
        }),
      );
    }
    const rows = await openIntegrationRows(testPage, seedData.workspaceId);
    for (const slug of ["github", "gitlab"]) {
      await expect(rows.locator(`a[href$="/integrations/${slug}"]`)).toContainText("Enabled");
    }

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/integrations`);
    for (const slug of ["github", "gitlab"]) await testPage.locator(`#${slug}-enabled`).tap();
    const save = testPage.getByTestId("settings-floating-save");
    await save.getByRole("button", { name: "Save changes" }).tap();
    await expect(save).not.toBeVisible();
    await openIntegrationRows(testPage, seedData.workspaceId);
    for (const slug of ["github", "gitlab"]) {
      const row = rows.locator(`a[href$="/integrations/${slug}"]`);
      await expect(row).toBeVisible();
      await expect(row).not.toContainText("Enabled");
    }
    await rows
      .locator('a[href$="/integrations/gitlab"]')
      .evaluate((row) => row.scrollIntoView({ block: "center" }));
    await testPage.evaluate(async () => {
      await Promise.allSettled(
        document
          .getAnimations()
          .filter(
            (animation) =>
              animation.playState === "running" &&
              animation.effect?.getTiming().iterations !== Infinity,
          )
          .map((animation) => animation.finished),
      );
    });
    await prCapture.screenshot("phone-disabled-integration-badges", {
      caption:
        "The phone Settings index keeps disabled GitHub and GitLab reachable without Enabled badges.",
    });

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/integrations`);
    for (const slug of ["github", "gitlab"]) await testPage.locator(`#${slug}-enabled`).tap();
    await save.getByRole("button", { name: "Save changes" }).tap();
    await expect(save).not.toBeVisible();
    await openIntegrationRows(testPage, seedData.workspaceId);
    for (const slug of ["github", "gitlab"]) {
      await expect(rows.locator(`a[href$="/integrations/${slug}"]`)).toContainText("Enabled");
    }
  });

  // @covers AC-UI-SETTINGS-MENU-DEFAULT-001.1
  test("renders accordion by default and navigates from the settings tree", async ({
    testPage,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings");

    const index = testPage.getByTestId("settings-index");
    await expect(index).toBeVisible();
    // Stays on /settings: no desktop-style handoff, and nothing to go Back past.
    await expect(testPage).toHaveURL(/\/settings$/);

    // Accordion is the default on the phone Settings index too.
    await index.getByRole("button", { name: /Expand Workspaces/ }).tap();
    await expect(
      index.locator(`a[href="/settings/workspaces/${seedData.workspaceId}"]`),
    ).toBeVisible();

    await index.getByRole("link", { name: /Terminal & Editors/ }).click();

    await expect(testPage).toHaveURL(/\/settings\/preferences\/terminal-editors$/);
    await expect(testPage.getByTestId("terminal-font-select")).toBeVisible();
  });

  test("offers no nav drawer anywhere in settings", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });

    for (const path of [
      "/settings",
      "/settings/preferences/terminal-editors",
      "/settings/prompts",
    ]) {
      await testPage.goto(path);
      await expect(testPage.getByTestId("settings-scroll-container")).toBeVisible();
      // A sheet here would offer the list the index page already is.
      await expect(testPage.getByTestId("app-nav-trigger")).toHaveCount(0);
    }
  });

  test("keeps the search field in thumb reach at the bottom of the index", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings");

    // The desktop sidebar's tree is in the DOM too (hidden below md) and carries
    // the same testid, so scope to the page's copy.
    const search = testPage.getByTestId("settings-index").getByTestId("settings-search");
    await expect(search).toBeVisible();

    const [box, viewport] = [
      await search.boundingBox(),
      await testPage.evaluate(() => ({ w: innerWidth, h: innerHeight })),
    ];
    expect(box).not.toBeNull();
    expect(box!.y).toBeGreaterThan(viewport.h / 2);
    expect(box!.y + box!.height).toBeLessThanOrEqual(viewport.h);

    // Centred, and clear of the config-chat button that shares that corner —
    // the field gives up width for both rather than trading one against the other.
    const centreOffset = Math.abs(box!.x + box!.width / 2 - viewport.w / 2);
    expect(centreOffset).toBeLessThanOrEqual(2);

    const chat = testPage.getByRole("button", { name: "Configuration Chat" });
    const chatBox = await chat.boundingBox();
    expect(chatBox).not.toBeNull();
    expect(box!.x + box!.width).toBeLessThanOrEqual(chatBox!.x);
    // Sharing a centre line with it, so the pair reads as one row.
    expect(box!.y + box!.height / 2).toBeCloseTo(chatBox!.y + chatBox!.height / 2, 0);

    // And no dead scroll under the last row: the index floats a search field,
    // not a settings form's Save action.
    const container = testPage.getByTestId("settings-scroll-container");
    const padding = await container.evaluate((el) => getComputedStyle(el).paddingBottom);
    expect(Number.parseFloat(padding)).toBeLessThan(120);

    // Still filters the list it floats over.
    await search.getByRole("searchbox", { name: "Search settings" }).fill("terminal font size");
    await expect(
      testPage.getByTestId("settings-index").getByTestId("settings-search-results"),
    ).toBeVisible();
  });
});

async function openIntegrationRows(page: Page, workspaceId: string) {
  await page.goto("/settings");
  const index = page.getByTestId("settings-index");
  await expect(index).toBeVisible();
  for (const label of ["Workspaces", "E2E Workspace", "Integrations"]) {
    const toggle = index.getByRole("button", { name: new RegExp(`^(Expand|Collapse) ${label}$`) });
    await expect(toggle).toBeVisible();
    if ((await toggle.getAttribute("aria-expanded")) === "false") await toggle.tap();
  }
  await expect(
    index.locator(`a[href="/settings/workspaces/${workspaceId}/integrations/gitlab"]`),
  ).toBeVisible();
  return index;
}
