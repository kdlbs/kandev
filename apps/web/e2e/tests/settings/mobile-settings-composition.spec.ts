import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

async function expectContained(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    await page.evaluate(() => document.documentElement.clientWidth),
  );
}

test.describe("Mobile settings composition", () => {
  test("task behavior keeps grouped rows inside one phone scroll region", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/task-behavior");

    const page = testPage.getByTestId("task-behavior-settings");
    const runtime = testPage.getByTestId("task-behavior-runtime");
    await expect(page).toBeVisible();
    await expect(testPage.getByTestId("task-behavior-group")).toHaveCount(3);
    await expect(runtime.locator("details")).not.toHaveAttribute("open", "");

    await runtime.locator("summary").tap();
    await expect(testPage.getByTestId("message-queue-settings")).toBeVisible();

    const summaryBox = await runtime.locator("summary").boundingBox();
    const queueBox = await testPage.getByTestId("message-queue-settings").boundingBox();
    const viewport = testPage.viewportSize();
    expect(summaryBox).not.toBeNull();
    expect(queueBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(queueBox!.x).toBeGreaterThanOrEqual(0);
    expect(queueBox!.x + queueBox!.width).toBeLessThanOrEqual(viewport!.width + 1);
    expect(summaryBox!.height).toBeGreaterThanOrEqual(44);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );

    const nestedScrollOwners = await page.evaluate(
      (root) =>
        Array.from(root.querySelectorAll("*")).filter((element) => {
          const overflow = getComputedStyle(element).overflowY;
          return overflow === "auto" || overflow === "scroll";
        }).length,
    );
    expect(nestedScrollOwners).toBe(0);
  });

  test("switch rows activate across their full coarse-pointer target", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/task-behavior");

    const row = testPage
      .getByTestId("task-behavior-group")
      .first()
      .locator('[data-settings-touch-target="true"]')
      .first();
    const toggle = row.getByRole("switch");
    const rowBox = await row.boundingBox();
    expect(rowBox).not.toBeNull();
    expect(rowBox!.height).toBeGreaterThanOrEqual(44);
    expect(rowBox!.width).toBeGreaterThanOrEqual(44);

    const initial = await toggle.getAttribute("data-state");
    await row.tap({ position: { x: 4, y: rowBox!.height / 2 } });
    await expect(toggle).toHaveAttribute(
      "data-state",
      initial === "checked" ? "unchecked" : "checked",
    );
  });

  test("preferences keep notification groups contained on a phone", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/notifications");

    await expect(testPage.locator('[data-settings-group="true"]').first()).toBeVisible();
    await expectContained(testPage);
  });

  test("agent executor groups remain reachable on a phone", async ({ testPage }) => {
    await testPage.goto("/settings/agents");
    await expect(testPage.getByTestId("installed-agents-actions")).toBeVisible();
    await expect(testPage.locator('[data-settings-group="true"]').first()).toBeVisible();
    await expectContained(testPage);

    await testPage.goto("/settings/executors");
    await expect(testPage.locator('[data-settings-group="true"]').last()).toBeVisible();
    await expectContained(testPage);
  });

  test("workspace integration groups remain contained beside mobile tabs", async ({
    testPage,
    seedData,
  }) => {
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    await expect(testPage.getByTestId("workspace-settings-shell")).toBeVisible();
    const repositoriesGroup = testPage.locator('[data-settings-group="true"]').first();
    await expect(repositoriesGroup).toBeVisible();
    await expect(
      repositoriesGroup.locator(':scope > [data-settings-group-card="true"]'),
    ).toHaveCount(0);
    await expectContained(testPage);

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/integrations/github`);
    await expect(testPage.getByTestId("workspace-settings-shell")).toBeVisible();
    await expect(testPage.locator('[data-settings-group="true"]').first()).toBeVisible();
    await expectContained(testPage);
  });

  test("system account groups remain reachable on a phone", async ({ testPage }) => {
    await testPage.goto("/settings/system/users");
    await expect(testPage.getByTestId("users-table-card")).toBeVisible();
    await expectContained(testPage);

    await testPage.goto("/settings/account/tokens");
    await expect(testPage.getByTestId("api-tokens-card")).toBeVisible();
    await expectContained(testPage);
  });
});
