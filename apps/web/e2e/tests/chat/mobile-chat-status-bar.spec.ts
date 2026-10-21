import type { Locator, Page } from "@playwright/test";
import { expect, test as base } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { AppState } from "../../../lib/state/store";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { openConversationUsageTask } from "./conversation-usage-helpers";
import {
  COMPOSER_HISTORY_PROMPT,
  revealTranscriptControls,
} from "../workflow/composer-action-layout-helpers";

const test = base.extend({
  prCapture: async ({ testPage }, use, testInfo) => {
    const capture = new PrAssetCapture(testPage, testInfo.file, { captureKey: testInfo.title });
    await use(capture);
    capture.flush();
  },
});

test.describe.configure({ retries: 0 });

let originalAutoScrollControl = true;
test.beforeEach(async ({ apiClient }) => {
  const { settings } = await apiClient.getUserSettings();
  originalAutoScrollControl = settings.show_transcript_auto_scroll_control ?? true;
});
test.afterEach(async ({ apiClient }) => {
  await apiClient.saveUserSettings({
    show_transcript_auto_scroll_control: originalAutoScrollControl,
  });
});

async function associatePR(api: ApiClient, taskId: string, state = "merged") {
  await api.mockGitHubAssociateTaskPR({
    task_id: taskId,
    owner: "test-org",
    repo: "test-repo",
    pr_number: 3948,
    pr_url: "https://github.com/test-org/test-repo/pull/3948",
    pr_title: "Mobile toolbar repair",
    head_branch: "feature/mobile-toolbar",
    base_branch: "main",
    author_login: "test-user",
    state,
  });
}

async function expectContainedTouchTargets(bar: Locator) {
  const bounds = (await bar.boundingBox())!;
  const boxes = await bar.locator("button:visible").evaluateAll((buttons) =>
    buttons.map((button) => {
      const { x, y, width, height } = button.getBoundingClientRect();
      return { x, y, width, height };
    }),
  );
  for (const [index, box] of boxes.entries()) {
    expect(box.width).toBeGreaterThanOrEqual(44);
    expect(box.height).toBeGreaterThanOrEqual(44);
    expect(box.x).toBeGreaterThanOrEqual(bounds.x);
    expect(box.x + box.width).toBeLessThanOrEqual(bounds.x + bounds.width + 1);
    for (const other of boxes.slice(index + 1)) {
      const overlapX =
        Math.min(box.x + box.width, other.x + other.width) - Math.max(box.x, other.x);
      const overlapY =
        Math.min(box.y + box.height, other.y + other.height) - Math.max(box.y, other.y);
      expect(overlapX <= 1 || overlapY <= 1).toBe(true);
    }
  }
  expect(
    await bar
      .page()
      .evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth),
  ).toBe(true);
}

async function expectSameControlRow(status: Locator, utility: Locator) {
  await expect
    .poll(async () => {
      const left = (await status.boundingBox())!;
      const right = (await utility.boundingBox())!;
      return Math.abs(left.y + left.height / 2 - right.y - right.height / 2);
    })
    .toBeLessThanOrEqual(1);
}

// @covers AC-UI-COMPOSER-ACTION-WRAP-001.6, .7
test("shares status and transcript controls when they fit", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await apiClient.saveUserSettings({ show_transcript_auto_scroll_control: true });
  const { taskId, session } = await openConversationUsageTask(
    testPage,
    apiClient,
    seedData,
    "Shared mobile composer row",
    'e2e:plan([{"content":"Review shared controls","status":"in_progress"}])\ne2e:message("Controls ready.")',
  );
  await associatePR(apiClient, taskId, "open");
  const bar = session.chatStatusBar();
  const todo = bar.getByTestId("todo-indicator");
  const ci = bar.getByTestId("pr-status-chip");
  const usage = bar.getByTestId("conversation-usage-trigger");
  await expect(todo).toBeVisible();
  await expect(ci).toBeVisible();
  await expect(usage).toBeVisible();
  for (const width of [393, 640, 767]) {
    await testPage.setViewportSize({ width, height: 851 });
    await expectSameControlRow(todo, usage);
    await expectSameControlRow(ci, usage);
    await expectContainedTouchTargets(bar);
  }
  await testPage.setViewportSize({ width: 393, height: 851 });
  await waitForFiniteAnimations(bar);
  await prCapture.screenshot("mobile-composer-shared-row", {
    caption: "Status icons and transcript utilities share a phone row",
  });
  await bar.getByTestId("proceed-next-step").focus();
  await testPage.keyboard.press("Tab");
  await expect(todo).toBeFocused();
  await testPage.keyboard.press("Tab");
  await expect(ci).toBeFocused();
  await todo.tap();
  await expect(testPage.getByTestId("todo-indicator-popover")).toContainText(
    "Review shared controls",
  );
  await testPage.keyboard.press("Escape");
  await ci.tap();
  await expect(testPage.getByTestId("pr-status-chip-drawer")).toBeVisible();
  await testPage.getByTestId("pr-status-chip-drawer-close").tap();
  await usage.tap();
  const drawer = testPage.getByTestId("conversation-usage-drawer");
  await expect(drawer).toContainText("1,200");
  await drawer.getByRole("button", { name: "Close" }).first().tap();
  await expect(usage).toBeFocused();
});

// @covers AC-UI-COMPOSER-ACTION-WRAP-001.7
test("wraps status and transcript controls without shrinking targets", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const { settings } = await apiClient.getUserSettings();
  try {
    await apiClient.saveUserSettings({
      show_transcript_auto_scroll_control: true,
      show_scroll_to_start: true,
      show_scroll_to_last_prompt: true,
    });
    const { taskId, session } = await openConversationUsageTask(
      testPage,
      apiClient,
      seedData,
      "Wrapped mobile composer row",
      'e2e:plan([{"content":"Review wrapping","status":"in_progress"}])\n' +
        COMPOSER_HISTORY_PROMPT,
    );
    await associatePR(apiClient, taskId, "open");
    await testPage.setViewportSize({ width: 320, height: 851 });
    await revealTranscriptControls(session.activeChat());
    const bar = session.chatStatusBar();
    const todo = bar.getByTestId("todo-indicator");
    const controls = bar.getByTestId("chat-status-bar-right-controls");
    await expect(todo).toBeVisible();
    await expect(bar.getByTestId("pr-status-chip")).toBeVisible();
    await expect
      .poll(async () => {
        const status = (await todo.boundingBox())!;
        const utilities = (await controls.boundingBox())!;
        return utilities.y >= status.y + status.height;
      })
      .toBe(true);
    await expectContainedTouchTargets(bar);
    await waitForFiniteAnimations(bar);
    await prCapture.screenshot("mobile-composer-wrapped-row", {
      caption: "Dense phone controls wrap without shrinking touch targets",
    });
    await bar.getByTestId("scroll-to-start-button").tap();
    await expect
      .poll(() =>
        session
          .activeChat()
          .locator(".chat-message-list")
          .evaluate((element) => element.scrollTop),
      )
      .toBe(0);
    await bar.getByTestId("jump-to-latest-button").tap();
    await expect(bar.getByTestId("jump-to-latest-button")).toBeHidden();
  } finally {
    await apiClient.saveUserSettings({
      show_scroll_to_start: settings.show_scroll_to_start === true,
      show_scroll_to_last_prompt: settings.show_scroll_to_last_prompt === true,
    });
  }
});

async function expectPhoneControls(page: Page, banner: Locator) {
  const bar = page.getByTestId("chat-status-bar");
  const barBox = (await bar.boundingBox())!;
  const bannerBox = (await banner.boundingBox())!;
  expect(bannerBox.width).toBeGreaterThanOrEqual(barBox.width - 1);
  await expect(banner).toHaveCSS("order", "-9999");
  const targets = bar.locator("button:visible");
  for (const control of await targets.all()) {
    const box = (await control.boundingBox())!;
    expect(box.height, await control.getAttribute("data-testid")).toBeGreaterThanOrEqual(44);
    expect(box.width).toBeGreaterThanOrEqual(44);
    expect(box.x).toBeGreaterThanOrEqual(barBox.x - 1);
    expect(box.x + box.width).toBeLessThanOrEqual(barBox.x + barBox.width + 1);
    expect(box.y + box.height).toBeLessThanOrEqual(barBox.y + barBox.height + 1);
  }
  for (const testId of ["share-task-button", "conversation-usage-trigger", "proceed-next-step"]) {
    const box = (await bar.getByTestId(testId).boundingBox())!;
    expect(box.y).toBeGreaterThanOrEqual(bannerBox.y + bannerBox.height);
  }
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}

async function removeBoardMembership(page: Page, taskId: string) {
  await page.evaluate((id) => {
    const store = (
      window as Window & {
        __KANDEV_E2E_STORE__?: {
          getState: () => AppState;
          setState: (update: (state: AppState) => void) => void;
        };
      }
    ).__KANDEV_E2E_STORE__;
    if (!store?.getState().taskOverview.byId[id]) {
      throw new Error("The active detail must retain its canonical task");
    }
    store.setState((state) => {
      state.kanban.tasks = state.kanban.tasks.filter((task) => task.id !== id);
      for (const snapshot of Object.values(state.kanbanMulti.snapshots)) {
        snapshot.tasks = snapshot.tasks.filter((task) => task.id !== id);
      }
    });
    if (!store.getState().taskOverview.byId[id]) {
      throw new Error("Removing board membership released the active detail's task");
    }
  }, taskId);
}

// @covers AC-UI-MOBILE-TASK-ACTION-BAR-001.1 through AC-UI-MOBILE-TASK-ACTION-BAR-001.8
// Geometry proves containment; actual actions prove the controls still work.
test("gives the terminal notice its own phone row with reachable actions", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(90_000);
  await apiClient.saveUserSettings({ show_transcript_auto_scroll_control: true });
  const { taskId, session } = await openConversationUsageTask(
    testPage,
    apiClient,
    seedData,
    "Mobile action bar repair",
    [
      'e2e:plan([{"content":"Review mobile controls","status":"in_progress"},{"content":"Capture screenshots","status":"pending"}])',
      'e2e:message("The mobile task controls are ready to review.")',
    ].join("\n"),
  );
  await associatePR(apiClient, taskId);
  const bar = session.chatStatusBar();
  const banner = session.prMergedBanner();
  await expect(banner).toContainText("PR #3948 merged");
  await expect(bar.getByTestId("todo-indicator")).toBeVisible();
  await expect(bar.getByTestId("conversation-usage-trigger")).toBeVisible();
  await expect(bar.getByTestId("proceed-next-step")).toBeVisible();
  await removeBoardMembership(testPage, taskId);
  await expect(bar.getByTestId("proceed-next-step")).toBeVisible();
  for (const width of [320, 393, 640, 767]) {
    await testPage.setViewportSize({ width, height: 851 });
    await expectPhoneControls(testPage, banner);
  }
  await testPage.setViewportSize({ width: 393, height: 851 });
  await waitForFiniteAnimations(bar);
  await prCapture.screenshot("mobile-task-action-bar", {
    caption: "Phone task notice with complete touch controls",
  });
  await bar.getByTestId("todo-indicator").tap();
  await expect(testPage.getByTestId("todo-indicator-popover")).toContainText(
    "Review mobile controls",
  );
  await testPage.keyboard.press("Escape");
  await bar.getByTestId("auto-scroll-toggle-button").tap();
  await expect(bar.getByTestId("auto-scroll-toggle-button")).toHaveAttribute(
    "aria-pressed",
    "false",
  );
  await bar.getByTestId("conversation-usage-trigger").tap();
  const usage = testPage.getByTestId("conversation-usage-drawer");
  await expect(usage).toContainText("1,200");
  await usage.getByRole("button", { name: "Close" }).first().tap();
  await expect(bar.getByTestId("conversation-usage-trigger")).toBeFocused();
  await bar.getByTestId("share-task-button").tap();
  const share = testPage.getByRole("dialog", { name: "Share this task" });
  await expect(share).toBeVisible();
  await testPage.keyboard.press("Escape");
  await session.prMergedArchiveButton().tap();
  await expect(session.prMergedArchiveConfirmButton()).toBeEnabled();
  await testPage.getByRole("button", { name: "Cancel", exact: true }).tap();
  await expect(banner).toBeVisible();
  await expect(testPage).toHaveURL(new RegExp(`/t/${taskId}`));
  await session.prMergedDismissButton().tap();
  await expect(banner).toBeHidden();
  await testPage.reload();
  await session.waitForLoad();
  await expect(banner).toBeHidden();
  await expect(bar.getByTestId("share-task-button")).toBeVisible();
  const sibling = await apiClient.seedTask(seedData.workspaceId, "Another mobile task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await testPage.goto(`/t/${sibling.task_id}`);
  await expect(testPage.getByTestId("pr-merged-banner")).toBeHidden();
  await testPage.goto(`/t/${taskId}`);
  await session.waitForLoad();
  await expect(banner).toBeHidden();
});

test("contains long translated closed notices and archives through confirmation", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(90_000);
  const { taskId, session } = await openConversationUsageTask(
    testPage,
    apiClient,
    seedData,
    "Closed mobile task",
  );
  await associatePR(apiClient, taskId, "closed");
  await expect(session.prClosedBanner()).toContainText("closed without merging");
  await testPage.setViewportSize({ width: 320, height: 851 });
  await expectPhoneControls(testPage, session.prClosedBanner());
  await testPage
    .context()
    .addCookies([{ name: "kandev_locale", value: "pt-pt", url: testPage.url() }]);
  await testPage.reload();
  await session.waitForLoad();
  await expect(session.prClosedBanner()).toContainText("fechado sem integração");
  await expectPhoneControls(testPage, session.prClosedBanner());
  await waitForFiniteAnimations(session.chatStatusBar());
  await prCapture.screenshot("mobile-task-action-bar-translated", {
    caption: "Narrow phone with a translated closed PR notice",
  });
  await testPage
    .context()
    .addCookies([{ name: "kandev_locale", value: "en", url: testPage.url() }]);
  await testPage.reload();
  await session.waitForLoad();
  await session.prClosedArchiveButton().tap();
  await expect(session.prClosedArchiveConfirmButton()).toBeEnabled();
  await session.prClosedArchiveConfirmButton().tap();
  await expect
    .poll(async () => {
      const response = await apiClient.rawRequest("GET", `/api/v1/tasks/${taskId}`);
      const task = await response.json();
      return Boolean(task.archived_at);
    })
    .toBe(true);
});

test("retains phone targets on fine pointers and compact desktop controls", async ({
  browser,
  backend,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(90_000);
  const context = await browser.newContext({
    baseURL: backend.baseUrl,
    viewport: { width: 767, height: 900 },
    hasTouch: false,
  });
  try {
    const page = await context.newPage();
    const { taskId, session } = await openConversationUsageTask(
      page,
      apiClient,
      seedData,
      "Fine pointer action bar",
    );
    await associatePR(apiClient, taskId);
    const bar = session.chatStatusBar();
    await expect(session.prMergedBanner()).toBeVisible();
    await expectPhoneControls(page, session.prMergedBanner());
    await page.setViewportSize({ width: 393, height: 851 });
    await expectPhoneControls(page, session.prMergedBanner());
    await waitForFiniteAnimations(bar);
    await prCapture.screenshot("mobile-task-action-bar-common", {
      page,
      caption: "Phone task controls with the compact merged notice",
    });
    for (const width of [768, 1280]) {
      await page.setViewportSize({ width, height: 900 });
      await expect(session.prMergedBanner()).toHaveCSS("order", "0");
      await expect(bar.getByTestId("share-task-button")).toHaveCSS("height", "24px");
      await expect(bar.getByTestId("proceed-next-step")).toHaveCSS("height", "24px");
      await expect(bar.getByTestId("proceed-next-step")).toHaveCSS("margin-left", "0px");
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);
    }
    await waitForFiniteAnimations(bar);
    await prCapture.screenshot("desktop-task-action-bar", {
      page,
      caption: "Desktop retains compact inline task controls",
    });
    await associatePR(apiClient, taskId, "open");
    await page.setViewportSize({ width: 393, height: 851 });
    await expect(bar.getByTestId("pr-status-chip")).toBeVisible();
    await expectSameControlRow(
      bar.getByTestId("pr-status-chip"),
      bar.getByTestId("conversation-usage-trigger"),
    );
    await expectContainedTouchTargets(bar);
  } finally {
    await context.close();
  }
});

test("wraps a long workflow action and preserves coarse tablet targets", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(90_000);
  const { taskId, session } = await openConversationUsageTask(
    testPage,
    apiClient,
    seedData,
    "Long workflow label",
  );
  await associatePR(apiClient, taskId);
  const proceed = session.proceedNextStepButton();
  await expect(proceed).toBeVisible();
  const nextName = (await proceed.innerText()).trim();
  const nextStep = seedData.steps.find((step) => step.name === nextName)!;
  expect(nextStep).toBeTruthy();
  const longName = "Review and complete the mobile task controls with the team. ".repeat(4).trim();
  try {
    const updated = await apiClient.rawRequest("PUT", `/api/v1/workflow/steps/${nextStep.id}`, {
      id: nextStep.id,
      name: longName,
    });
    expect(updated.ok).toBe(true);
    await testPage.reload();
    await session.waitForLoad();
    await testPage.setViewportSize({ width: 320, height: 851 });
    await expect(proceed).toHaveAccessibleName(longName);
    await expectPhoneControls(testPage, session.prMergedBanner());
    await expect(proceed).toHaveCSS("white-space", "normal");
    await testPage.setViewportSize({ width: 800, height: 900 });
    await expect(proceed).toHaveCSS("white-space", "normal");
    expect(
      await proceed.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
    ).toBe(true);
    for (const control of [
      proceed,
      session.prMergedArchiveButton(),
      session.prMergedDismissButton(),
      testPage.getByTestId("share-task-button"),
    ]) {
      expect((await control.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    }
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await waitForFiniteAnimations(session.chatStatusBar());
    await prCapture.screenshot("tablet-task-action-bar-long-label", {
      caption: "Touch tablet keeps a long workflow label inside its action",
    });
  } finally {
    const restored = await apiClient.rawRequest("PUT", `/api/v1/workflow/steps/${nextStep.id}`, {
      id: nextStep.id,
      name: nextName,
    });
    expect(restored.ok).toBe(true);
  }
});
