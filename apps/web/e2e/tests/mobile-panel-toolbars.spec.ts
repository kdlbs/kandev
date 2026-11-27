import { expect, test } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";
import { GitHelper, makeGitEnv } from "../helpers/git-helper";
import { waitForFiniteAnimations } from "../helpers/animations";
import { waitForSessionDone } from "../helpers/session";
import {
  openHistoryRegression,
  seedHistoryRelation,
} from "./git/changes-history-regression-helpers";
import {
  expectFileBrowserIconCentered,
  readTaskWorkspacePath,
} from "../helpers/panel-toolbar-geometry";

async function createToolbarTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Toolbar task has no session");
  await waitForSessionDone(
    apiClient,
    task.id,
    task.session_id,
    "Waiting for the toolbar task's initial turn",
    45_000,
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  return session;
}

async function expectTouchHeaders(page: Page, surface: string) {
  await expect
    .poll(
      () =>
        page.locator("[data-panel-header]:visible").evaluateAll((headers) =>
          headers.map((header) => {
            const rect = header.getBoundingClientRect();
            return {
              height: rect.height,
              bottom: rect.bottom,
              nextTop: header.nextElementSibling?.getBoundingClientRect().top ?? null,
              wrap: window.getComputedStyle(header).flexWrap,
            };
          }),
        ),
      { timeout: 15_000, message: `Waiting for a touch header in ${surface}` },
    )
    .not.toEqual([]);

  const headers = await page.locator("[data-panel-header]:visible").evaluateAll((items) =>
    items.map((header) => {
      const rect = header.getBoundingClientRect();
      return {
        height: rect.height,
        bottom: rect.bottom,
        nextTop: header.nextElementSibling?.getBoundingClientRect().top ?? null,
        wrap: window.getComputedStyle(header).flexWrap,
      };
    }),
  );
  expect(headers.length, `expected touch header in ${surface}`).toBeGreaterThan(0);
  for (const header of headers) {
    expect(Math.abs(header.height - 48), `${surface} header height`).toBeLessThanOrEqual(1);
    expect(header.wrap).toBe("nowrap");
    if (header.nextTop !== null) {
      expect(
        Math.abs(header.nextTop - header.bottom),
        `${surface} content edge`,
      ).toBeLessThanOrEqual(1);
    }
  }
}

async function expectNoOverflow(page: Page, surface: string) {
  await expect
    .poll(
      () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1),
      { timeout: 5_000, message: `${surface} has horizontal overflow` },
    )
    .toBe(true);
}

async function expectTouchControls(page: Page, surface: string) {
  const controls = await page.locator("[data-panel-header]:visible").evaluateAll((headers) => {
    const measurements: Array<{
      label: string;
      tagName: string;
      height: number;
      width: number;
    }> = [];
    for (const header of headers) {
      const candidates = header.querySelectorAll<HTMLElement>(
        "button, a, input, select, textarea, [role='button']",
      );
      for (const control of candidates) {
        const style = window.getComputedStyle(control);
        if (style.display === "none" || style.visibility === "hidden") continue;
        const rect = control.getBoundingClientRect();
        if (rect.width === 0 || rect.height === 0) continue;
        measurements.push({
          label: control.getAttribute("aria-label") ?? control.textContent ?? control.tagName,
          tagName: control.tagName,
          height: rect.height,
          width: rect.width,
        });
      }
    }
    return measurements;
  });

  expect(controls, `${surface} should render touch controls`).not.toEqual([]);
  for (const control of controls) {
    expect(control.height, `${surface} ${control.label} height`).toBeGreaterThanOrEqual(44);
    if (!new Set(["INPUT", "SELECT", "TEXTAREA"]).has(control.tagName)) {
      expect(control.width, `${surface} ${control.label} width`).toBeGreaterThanOrEqual(44);
    }
  }
}

test.describe("touch panel toolbars", () => {
  test("keeps Review and Diff reachable on a narrow phone with diverged history", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 320, height: 844 });
    await openHistoryRegression(testPage, apiClient, seedData, true);
    await seedHistoryRelation(testPage, "diverged");
    const changes = testPage.getByTestId("mobile-changes-panel");
    const warning = changes.getByTestId("header-remote-contribution-warning");
    await expect(warning).toBeVisible();
    const review = changes.getByRole("button", { name: "Review", exact: true });
    const overflow = changes.getByTestId("panel-header-overflow");
    for (const control of [review, warning, overflow]) {
      await expect
        .poll(() =>
          control.evaluate((element) => {
            const box = element.getBoundingClientRect();
            return element.contains(
              document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
            );
          }),
        )
        .toBe(true);
    }
    await overflow.tap();
    await expect(testPage.getByRole("menu").getByRole("menuitem")).toHaveCount(2);
    await testPage.keyboard.press("Escape");
    await review.tap();
    await expect(
      testPage.getByRole("dialog", { name: "Review Changes", exact: true }),
    ).toBeVisible();
  });

  test("shows only hidden Changes actions in the phone overflow menu", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    await createToolbarTask(testPage, apiClient, seedData, "Phone Changes overflow");
    const git = new GitHelper(seedData.repositoryPath, makeGitEnv(backend.tmpDir));
    git.createFile("phone-toolbar-actions.ts", "export const phoneToolbarActions = true;\n");
    await testPage
      .getByRole("navigation")
      .getByRole("button", { name: /Changes$/ })
      .tap();
    const changes = testPage.getByTestId("mobile-changes-panel");
    const diff = changes.getByRole("button", { name: "Diff", exact: true });
    const overflow = changes.getByTestId("panel-header-overflow");
    await expect(diff).toBeVisible({ timeout: 15_000 });
    await expect(overflow).toHaveCount(0);
    await expect(changes.getByRole("button", { name: "Review", exact: true })).toBeVisible();
    await expect(changes.getByTestId("changes-request-walkthrough")).toBeVisible();
    await prCapture.screenshot("changes-inline-actions", {
      caption: "Phone Changes keeps Diff, Review and Walkthrough visible without a duplicate menu.",
    });

    await testPage.setViewportSize({ width: 340, height: 844 });
    await expect(diff).toBeHidden();
    await expect(overflow).toBeVisible();
    const box = await overflow.boundingBox();
    expect(box?.width).toBeGreaterThanOrEqual(44);
    expect(box?.height).toBeGreaterThanOrEqual(44);
    await overflow.tap();
    const menu = testPage.getByRole("menu");
    await expect(menu.getByRole("menuitem")).toHaveCount(2);
    await expect(changes.getByTestId("changes-request-walkthrough")).toBeHidden();
    await waitForFiniteAnimations(menu);
    await prCapture.screenshot("changes-diff-overflow", {
      caption:
        "A narrow phone keeps Diff and Walkthrough reachable through a menu without duplicating Review.",
    });
    await menu.getByRole("menuitem", { name: "Diff", exact: true }).tap();
    await expect(testPage.getByRole("dialog")).toBeVisible();
    await expectNoOverflow(testPage, "phone Changes overflow");
  });

  test("centers Files copy path icons before and after copying", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
    await testPage.context().grantPermissions(["clipboard-read", "clipboard-write"]);

    await createToolbarTask(testPage, apiClient, seedData, "Phone copy path icon alignment");
    await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    await expect(testPage.getByTestId("file-tree-scroll")).toBeVisible();

    const filesPanel = testPage.getByTestId("files-panel");
    const copyButton = filesPanel.getByRole("button", {
      name: "Copy workspace path",
      exact: true,
    });
    await expect(copyButton).toBeVisible({ timeout: 20_000 });
    const buttonBox = await copyButton.boundingBox();
    expect(buttonBox).not.toBeNull();
    expect(buttonBox!.width).toBeGreaterThanOrEqual(44);
    expect(buttonBox!.height).toBeGreaterThanOrEqual(44);
    await expectFileBrowserIconCentered(copyButton, 0, "normal phone");

    const expectedPath = await readTaskWorkspacePath(testPage, apiClient);
    expect(expectedPath.startsWith("/")).toBe(true);
    // Touch has no hover state; tapping copies directly and reveals the check glyph.
    await copyButton.tap();
    await expectFileBrowserIconCentered(copyButton, 1, "copied phone");
    await expect
      .poll(() => testPage.evaluate(() => navigator.clipboard.readText()))
      .toBe(expectedPath);

    const pathLabel = filesPanel.getByTestId("file-browser-workspace-path");
    const [target, path] = await Promise.all([copyButton.boundingBox(), pathLabel.boundingBox()]);
    expect(target).not.toBeNull();
    expect(path).not.toBeNull();
    expect(target!.x + target!.width).toBeLessThanOrEqual(path!.x + 1);
    await expectNoOverflow(testPage, "Files copy path");
  });

  test("keeps the 390px phone panels contained and preserves task navigation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);

    const session = await createToolbarTask(testPage, apiClient, seedData, "Panel toolbar phone");
    await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    await expect(testPage.getByTestId("file-tree-scroll")).toBeVisible();
    await expectTouchHeaders(testPage, "390px Files");
    await expectTouchControls(testPage, "390px Files");

    const searchButton = testPage.getByRole("button", { name: "Search files", exact: true });
    await expect(searchButton).toBeVisible();
    expect((await searchButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await expectNoOverflow(testPage, "390px Files");

    await testPage.getByRole("button", { name: /Changes/ }).tap();
    await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible();
    await expectTouchHeaders(testPage, "390px Changes");
    await expectTouchControls(testPage, "390px Changes");
    await expectNoOverflow(testPage, "390px Changes");

    await testPage.getByRole("button", { name: "Chat", exact: true }).tap();
    await expect(session.activeChat()).toBeVisible();
  });

  test("keeps 767px and 900px coarse headers at touch geometry", async ({
    testPage,
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 767, height: 900 });
    await createToolbarTask(testPage, apiClient, seedData, "Panel toolbar coarse boundaries");
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
    await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    await expect(testPage.getByTestId("file-tree-scroll")).toBeVisible();
    await expectTouchHeaders(testPage, "767px Files");
    await expectTouchControls(testPage, "767px Files");
    await expectNoOverflow(testPage, "767px Files");

    const taskHref = await testPage.url();
    await tabletTestPage.goto(taskHref);
    const tabletSession = new SessionPage(tabletTestPage);
    await tabletSession.waitForLoad();
    await tabletSession.waitForChatIdle({ timeout: 45_000 });
    expect(await tabletTestPage.evaluate(() => window.innerWidth)).toBe(900);
    expect(await tabletTestPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
    await expect(tabletTestPage.getByTestId("file-tree-scroll")).toBeVisible();
    await expectTouchHeaders(tabletTestPage, "900px Files");
    await expectTouchControls(tabletTestPage, "900px Files");
    await expectNoOverflow(tabletTestPage, "900px Files");
  });
});
