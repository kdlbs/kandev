import fs from "node:fs/promises";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  installFixturePlugin,
  PLUGIN_ID,
  uninstallFixturePlugin,
} from "../../helpers/plugin-fixture";
import { openPhoneList, SIDEBAR_QUERY_PATH } from "./mobile-list-helpers";
import {
  readPreviousSidebarColorPatch,
  readPreviousSidebarViewState,
  restoreSidebarSortColors,
  restoreSidebarViewState,
} from "./sidebar-running-first-activity-sort-helpers";

const EVIDENCE_DIR = path.resolve(
  __dirname,
  "../../../../../docs/plans/mobile-list-layout/evidence",
);
const MEASURED_SELECTORS = [
  "mobile-topbar-page-context",
  "mobile-list-search-toggle",
  "app-nav-trigger",
  "sidebar-view-chip-row",
  "mobile-list-search-row",
  "mobile-tasks-list-scroll",
  "mobile-list-view-editor-scroll",
  "app-nav-sheet",
  "mobile-list-result-filters-drawer",
  "mobile-list-filter-empty-state",
  "mobile-list-link-scope",
  "mobile-list-facet-sort",
  "mobile-list-facet-group",
  "mobile-menu-drawer-content",
  "tasks-list-search",
  "sidebar-task-item",
] as const;

async function capturePreview(
  page: import("@playwright/test").Page,
  testInfo: import("@playwright/test").TestInfo,
  name: string,
) {
  await waitForFiniteAnimations(page.locator("body"));
  const outputPath = testInfo.outputPath(`${name}.png`);
  await page.screenshot({ path: outputPath, animations: "disabled", caret: "hide" });
  const geometry = await page.evaluate(
    (selectors) => {
      const elements = Object.fromEntries(
        selectors.map((testId) => {
          const element = document.querySelector(`[data-testid="${testId}"]`);
          if (!element) return [testId, null];
          const rect = element.getBoundingClientRect();
          const style = getComputedStyle(element);
          const hit = document.elementFromPoint(
            rect.left + rect.width / 2,
            rect.top + rect.height / 2,
          );
          return [
            testId,
            {
              x: rect.x,
              y: rect.y,
              width: rect.width,
              height: rect.height,
              minHeight: style.minHeight,
              overflowX: style.overflowX,
              overflowY: style.overflowY,
              clientHeight: element.clientHeight,
              scrollHeight: element.scrollHeight,
              centerHit: hit === element || element.contains(hit),
              insideViewport:
                rect.left >= 0 &&
                rect.top >= 0 &&
                rect.right <= window.innerWidth &&
                rect.bottom <= window.innerHeight,
            },
          ];
        }),
      );
      return {
        viewport: { width: window.innerWidth, height: window.innerHeight },
        document: {
          width: document.documentElement.scrollWidth,
          height: document.documentElement.scrollHeight,
          horizontalOverflow: document.documentElement.scrollWidth > window.innerWidth,
        },
        elements,
      };
    },
    [...MEASURED_SELECTORS],
  );
  const geometryPath = testInfo.outputPath(`${name}.json`);
  await fs.writeFile(geometryPath, `${JSON.stringify(geometry, null, 2)}\n`);
  const capturePrAssets = process.env.CAPTURE_PR_ASSETS === "true";
  if (capturePrAssets) {
    await fs.mkdir(EVIDENCE_DIR, { recursive: true });
    await fs.copyFile(outputPath, path.join(EVIDENCE_DIR, `${name}.png`));
    await fs.copyFile(geometryPath, path.join(EVIDENCE_DIR, `${name}.json`));
  }
  await testInfo.attach(name, { path: outputPath, contentType: "image/png" });
  await testInfo.attach(`${name}-geometry`, {
    path: geometryPath,
    contentType: "application/json",
  });
  console.log(
    `VISUAL_ACCEPTANCE ${JSON.stringify({ name, path: capturePrAssets ? EVIDENCE_DIR : geometryPath, ...geometry })}`,
  );
  expect(
    (geometry as { document: { horizontalOverflow: boolean } }).document.horizontalOverflow,
  ).toBe(false);
}

async function setPhoneTheme(page: import("@playwright/test").Page, dark: boolean) {
  const html = page.locator("html");
  const current = await html.evaluate((element) => element.classList.contains("dark"));
  if (current === dark) return;
  await page.getByTestId("app-nav-trigger").tap();
  const themeToggle = page.getByTestId("mobile-theme-toggle-button");
  await themeToggle.scrollIntoViewIfNeeded();
  await themeToggle.tap();
  await expect(html).toHaveClass(dark ? /dark/ : /light/);
  await page.keyboard.press("Escape");
}

test.describe.configure({ retries: 0 });

test("captures phone List previews and linked plugin overrides", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(180_000);
  const token = `mobile-preview-${Date.now()}`;
  const previousSettings = await apiClient.getUserSettings();
  const previousViews = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const previousColors = await readPreviousSidebarColorPatch(apiClient);
  const originalWorkspaceId = previousSettings.settings.workspace_id;
  const originalLayout = previousSettings.settings.kanban_view_mode;
  const taskOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const running = await apiClient.createTask(seedData.workspaceId, `${token} running`, taskOptions);
  const activity = await apiClient.createTask(
    seedData.workspaceId,
    `${token} recent activity`,
    taskOptions,
  );
  const idle = await apiClient.createTask(seedData.workspaceId, `${token} idle`, taskOptions);
  const { session_id: runningSession } = await apiClient.seedTaskSession(running.id, {
    state: "RUNNING",
    startedAt: new Date(Date.now() - 120_000).toISOString(),
  });
  await apiClient.setPrimarySession(runningSession);
  await apiClient.updateTaskTitle(activity.id, `${token} newest activity`);
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seedData.workspaceId,
      views: [
        {
          id: "view-all-tasks",
          name: "All tasks",
          filters: [],
          sort: { key: "state", direction: "asc" },
          group: "none",
          group_indent: true,
          collapsed_groups: [],
        },
        {
          id: `${token}-archived`,
          name: "Archived",
          filters: [
            { id: `${token}-archive-filter`, dimension: "archived", op: "is", value: true },
          ],
          sort: { key: "state", direction: "asc" },
          group: "none",
          group_indent: true,
          collapsed_groups: [],
        },
        {
          id: `${token}-view`,
          name: "My view",
          filters: [],
          sort: {
            key: "running",
            direction: "desc",
            then_by: [
              { key: "color", color: "red", direction: "desc" },
              { key: "color", color: "orange", direction: "desc" },
              { key: "lastActivityAt", direction: "desc" },
            ],
          },
          group: "none",
          group_indent: true,
          collapsed_groups: [],
        },
      ],
      active_view_id: `${token}-view`,
      draft: null,
    },
    sidebar_task_color_patch: {
      colors: { [running.id]: "red", [activity.id]: "orange", [idle.id]: "blue" },
      if_missing: false,
    },
  });
  await testPage.addInitScript(() => {
    if (window.name !== "mobile-list-preview-theme-initialized") {
      localStorage.setItem("theme", "light");
      window.name = "mobile-list-preview-theme-initialized";
    }
  });

  try {
    await openPhoneList(testPage);
    await expect(testPage.getByTestId("sidebar-view-chip-row")).toBeVisible();
    await expect(testPage.getByTestId("sidebar-task-item")).toHaveCount(3);
    await capturePreview(testPage, testInfo, "ui-01-phone-list-light-393x852");

    await testPage.getByTestId("mobile-list-search-toggle").tap();
    await expect(testPage.getByTestId("mobile-list-search-input")).toBeFocused();
    await capturePreview(testPage, testInfo, "ui-02-phone-search-light-393x852");
    await testPage.getByTestId("mobile-list-result-filters").tap();
    await expect(testPage.getByTestId("mobile-list-result-filters-drawer")).toBeVisible();
    await capturePreview(testPage, testInfo, "ui-03-phone-result-filters-light-393x852");
    await testPage.keyboard.press("Escape");
    await expect(testPage.getByTestId("mobile-list-result-filters-drawer")).toHaveCount(0);
    await testPage.getByTestId("mobile-list-search-toggle").tap();
    await expect(testPage.getByTestId("mobile-list-search-row")).toHaveCount(0);

    await testPage.getByTestId("mobile-topbar-page-context").tap();
    const options = testPage.getByRole("dialog", { name: "View options" });
    await expect(options.getByTestId("mobile-list-view-editor")).toBeVisible();
    const lightDrawer = await testPage.getByTestId("mobile-menu-drawer-content").boundingBox();
    expect(lightDrawer?.x).toBeGreaterThanOrEqual(8);
    expect(lightDrawer?.width).toBeLessThanOrEqual(393 - 16);
    expect(lightDrawer?.height).toBeLessThanOrEqual(852 * 0.75);
    await capturePreview(testPage, testInfo, "ui-04-phone-view-options-light-393x852");
    await options.getByTestId("sidebar-sort-settings-toggle").tap();
    await expect(options.getByTestId("sidebar-sort-chain-editor")).toBeVisible();
    await capturePreview(testPage, testInfo, "ui-05-phone-four-rule-sort-light-393x852");
    await options
      .getByTestId("mobile-list-view-editor-scroll")
      .evaluate((element) => (element.scrollTop = element.scrollHeight));
    await capturePreview(testPage, testInfo, "ui-05-phone-four-rule-sort-light-lower-393x852");
    await testPage.keyboard.press("Escape");
    await expect(options).toHaveCount(0);

    await testPage.getByTestId("app-nav-trigger").tap();
    const lightNav = testPage.getByTestId("app-nav-sheet");
    await expect(lightNav).toBeVisible();
    await capturePreview(testPage, testInfo, "ui-06-phone-navigation-light-393x852");
    await lightNav.getByTestId("mobile-navigation-tasks-toggle").scrollIntoViewIfNeeded();
    await expect(lightNav.getByTestId("mobile-navigation-tasks-toggle")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await expect(lightNav.getByTestId("mobile-list-view-editor")).toHaveCount(0);
    await capturePreview(testPage, testInfo, "ui-06-phone-navigation-light-lower-393x852");
    await testPage.keyboard.press("Escape");
    await expect(lightNav).toHaveCount(0);

    await setPhoneTheme(testPage, true);
    await expect(testPage.getByTestId("sidebar-view-chip-row")).toBeVisible();
    await capturePreview(testPage, testInfo, "ui-01-phone-list-dark-393x852");

    await testPage.getByTestId("mobile-list-search-toggle").tap();
    await expect(testPage.getByTestId("mobile-list-search-input")).toBeFocused();
    await capturePreview(testPage, testInfo, "ui-02-phone-search-dark-393x852");
    await testPage.getByTestId("mobile-list-result-filters").tap();
    await expect(testPage.getByTestId("mobile-list-result-filters-drawer")).toBeVisible();
    await capturePreview(testPage, testInfo, "ui-03-phone-result-filters-dark-393x852");
    await testPage.keyboard.press("Escape");
    await expect(testPage.getByTestId("mobile-list-result-filters-drawer")).toHaveCount(0);
    await testPage.getByTestId("mobile-list-search-toggle").tap();
    await expect(testPage.getByTestId("mobile-list-search-row")).toHaveCount(0);

    await testPage.getByTestId("mobile-topbar-page-context").tap();
    const darkOptions = testPage.getByRole("dialog", { name: "View options" });
    await expect(darkOptions.getByTestId("mobile-list-view-editor")).toBeVisible();
    const collapsedDrawer = await testPage.getByTestId("mobile-menu-drawer-content").boundingBox();
    expect(collapsedDrawer?.x).toBeGreaterThanOrEqual(8);
    expect(collapsedDrawer?.width).toBeLessThanOrEqual(393 - 16);
    expect(collapsedDrawer?.height).toBeLessThanOrEqual(852 * 0.75);
    await expect(darkOptions.getByTestId("mobile-list-view-editor-scroll")).toHaveCSS(
      "overflow-y",
      "auto",
    );
    await capturePreview(testPage, testInfo, "ui-04-phone-view-options-dark-393x852");
    await darkOptions.getByTestId("sidebar-sort-settings-toggle").tap();
    await expect(darkOptions.getByTestId("sidebar-sort-chain-editor")).toBeVisible();
    const scrollMetrics = await darkOptions
      .getByTestId("mobile-list-view-editor-scroll")
      .evaluate((element) => ({ client: element.clientHeight, scroll: element.scrollHeight }));
    expect(scrollMetrics.scroll).toBeGreaterThan(scrollMetrics.client);
    await capturePreview(testPage, testInfo, "ui-05-phone-four-rule-sort-dark-393x852");
    await darkOptions
      .getByTestId("mobile-list-view-editor-scroll")
      .evaluate((element) => (element.scrollTop = element.scrollHeight));
    await capturePreview(testPage, testInfo, "ui-05-phone-four-rule-sort-dark-lower-393x852");
    await testPage.keyboard.press("Escape");
    await expect(darkOptions).toHaveCount(0);

    await testPage.getByTestId("app-nav-trigger").tap();
    const darkNav = testPage.getByTestId("app-nav-sheet");
    await expect(darkNav).toBeVisible();
    await capturePreview(testPage, testInfo, "ui-06-phone-navigation-dark-393x852");
    await darkNav.getByTestId("mobile-navigation-tasks-toggle").scrollIntoViewIfNeeded();
    await expect(darkNav.getByTestId("mobile-navigation-tasks-toggle")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await capturePreview(testPage, testInfo, "ui-06-phone-navigation-dark-lower-393x852");
    await testPage.keyboard.press("Escape");
    await expect(darkNav).toHaveCount(0);

    await testPage.getByTestId("mobile-list-search-toggle").tap();
    await testPage.getByTestId("mobile-list-search-input").fill(`${token}-no-match`);
    await expect(testPage.getByTestId("mobile-list-filter-empty-state")).toBeVisible();
    await expect(testPage.getByTestId("mobile-list-clear-results")).toBeVisible();
    await capturePreview(testPage, testInfo, "ui-08-phone-empty-results-dark-393x852");

    await setPhoneTheme(testPage, false);
    await installFixturePlugin(testPage);
    await testPage.goto(
      `/tasks?workspace=${seedData.workspaceId}&workflow=all&repository=all&query=Fixture&sort=${encodeURIComponent(`facet:${PLUGIN_ID}:fixture-tags`)}&group=${encodeURIComponent(`facet:${PLUGIN_ID}:fixture-tags`)}`,
    );
    const lightSearchToggle = testPage.getByTestId("mobile-list-search-toggle");
    if ((await lightSearchToggle.getAttribute("aria-expanded")) !== "true") {
      await lightSearchToggle.tap();
    }
    await expect(testPage.getByTestId("mobile-list-link-scope")).toBeVisible();
    await capturePreview(testPage, testInfo, "ui-09-phone-link-scope-light-393x852");
    await testPage.getByTestId("mobile-topbar-page-context").tap();
    const linkedOptions = testPage.getByRole("dialog", { name: "View options" });
    await linkedOptions.getByTestId("mobile-list-additional-options-toggle").tap();
    const linkedFacetSort = linkedOptions.getByTestId("mobile-list-facet-sort");
    await expect(linkedFacetSort).toContainText("Fixture tag");
    const linkedFacetGroup = linkedOptions.getByTestId("mobile-list-facet-group");
    await expect(linkedFacetGroup).toContainText("Fixture tag");
    await linkedFacetSort.scrollIntoViewIfNeeded();
    await capturePreview(testPage, testInfo, "ui-09-phone-plugin-link-override-light-393x852");
    await linkedFacetGroup.scrollIntoViewIfNeeded();
    await capturePreview(testPage, testInfo, "ui-09-phone-plugin-group-override-light-393x852");
    await testPage.keyboard.press("Escape");

    await setPhoneTheme(testPage, true);
    await testPage.goto(
      `/tasks?workspace=${seedData.workspaceId}&workflow=all&repository=all&query=Fixture&sort=${encodeURIComponent(`facet:${PLUGIN_ID}:fixture-tags`)}&group=${encodeURIComponent(`facet:${PLUGIN_ID}:fixture-tags`)}`,
    );
    const darkSearchToggle = testPage.getByTestId("mobile-list-search-toggle");
    if ((await darkSearchToggle.getAttribute("aria-expanded")) !== "true") {
      await darkSearchToggle.tap();
    }
    await expect(testPage.getByTestId("mobile-list-link-scope")).toBeVisible();
    await capturePreview(testPage, testInfo, "ui-09-phone-link-scope-dark-393x852");
    await testPage.getByTestId("mobile-topbar-page-context").tap();
    const darkLinkedOptions = testPage.getByRole("dialog", { name: "View options" });
    await darkLinkedOptions.getByTestId("mobile-list-additional-options-toggle").tap();
    const darkLinkedFacetSort = darkLinkedOptions.getByTestId("mobile-list-facet-sort");
    await expect(darkLinkedFacetSort).toContainText("Fixture tag");
    const darkLinkedFacetGroup = darkLinkedOptions.getByTestId("mobile-list-facet-group");
    await expect(darkLinkedFacetGroup).toContainText("Fixture tag");
    await darkLinkedFacetSort.scrollIntoViewIfNeeded();
    await capturePreview(testPage, testInfo, "ui-09-phone-plugin-link-override-dark-393x852");
    await darkLinkedFacetGroup.scrollIntoViewIfNeeded();
    await capturePreview(testPage, testInfo, "ui-09-phone-plugin-group-override-dark-393x852");
  } finally {
    await uninstallFixturePlugin(apiClient);
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previousViews);
    await restoreSidebarSortColors(apiClient, [running.id, activity.id, idle.id], previousColors);
    await apiClient.saveUserSettings({
      workspace_id: originalWorkspaceId ?? seedData.workspaceId,
      ...(originalLayout === undefined ? {} : { kanban_view_mode: originalLayout }),
    });
  }
});

test("captures phone List loading and failed-read feedback", async ({ testPage }, testInfo) => {
  test.setTimeout(120_000);
  let releaseQuery: (() => void) | undefined;
  const queryGate = new Promise<void>((resolve) => {
    releaseQuery = resolve;
  });
  await testPage.setViewportSize({ width: 393, height: 852 });
  await testPage.addInitScript(() => localStorage.setItem("theme", "light"));
  await testPage.route(SIDEBAR_QUERY_PATH, async (route) => {
    await queryGate;
    await route.continue();
  });

  try {
    await testPage.goto("/tasks?query=preview-loading-state");
    const list = testPage.getByTestId("mobile-tasks-list-scroll");
    await expect(list.locator(".animate-pulse")).toBeVisible();
    await capturePreview(testPage, testInfo, "ui-08-phone-loading-light-393x852");
    releaseQuery?.();
    await expect(list.locator(".animate-pulse")).toHaveCount(0);
    await testPage.unroute(SIDEBAR_QUERY_PATH);
    await testPage.route(SIDEBAR_QUERY_PATH, (route) =>
      route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "Preview fixture failure" }),
      }),
    );
    await testPage.goto("/tasks?query=preview-error-state");
    await expect(testPage.getByTestId("sidebar-task-page-load-error")).toBeVisible();
    await capturePreview(testPage, testInfo, "ui-08-phone-error-light-393x852");
  } finally {
    releaseQuery?.();
    await testPage.unroute(SIDEBAR_QUERY_PATH);
  }
});

test("captures long-name containment and the coarse phone breakpoint", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const previousViews = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
  const previousWorkspace = (await apiClient.listWorkspaces()).workspaces.find(
    (workspace) => workspace.id === seedData.workspaceId,
  );
  const workspaceName = "Workspace with a deliberately long name for narrow phone headers";
  const prefix = `narrow-${Date.now()}`;
  const views = [
    {
      id: "view-all-tasks",
      name: "All tasks",
      filters: [],
      sort: { key: "state", direction: "asc" },
      group: "none",
      group_indent: true,
      collapsed_groups: [],
    },
    ...Array.from({ length: 10 }, (_, index) => ({
      id: `${prefix}-${index}`,
      name: `Long saved workspace view name ${String(index + 1).padStart(2, "0")}`,
      filters: [],
      sort: { key: "state", direction: "asc" },
      group: "none",
      group_indent: true,
      collapsed_groups: [],
    })),
  ];

  try {
    await apiClient.updateWorkspace(seedData.workspaceId, { name: workspaceName });
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        views,
        active_view_id: `${prefix}-9`,
        draft: null,
      },
    });
    await testPage.addInitScript(() => localStorage.setItem("theme", "light"));
    await testPage.setViewportSize({ width: 360, height: 800 });
    await testPage.goto(`/tasks?workspace=${seedData.workspaceId}`);
    const tabs = testPage.getByTestId("sidebar-view-chip-row");
    await expect(tabs.getByTestId("sidebar-view-chip")).toHaveCount(11);
    await expect(testPage.getByTestId("mobile-topbar-page-context")).toBeVisible();
    await expect(testPage.getByTestId("mobile-list-search-toggle")).toBeVisible();
    const tabMetrics = await tabs.evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
    }));
    expect(tabMetrics.scrollWidth).toBeGreaterThan(tabMetrics.clientWidth);
    await tabs.evaluate((element) => (element.scrollLeft = element.scrollWidth));
    await capturePreview(testPage, testInfo, "ui-01-phone-long-names-360x800");

    await testPage.setViewportSize({ width: 767, height: 900 });
    await expect(testPage.getByTestId("mobile-topbar-page-context")).toBeVisible();
    await expect(tabs).toBeVisible();
    await capturePreview(testPage, testInfo, "ui-01-phone-breakpoint-767x900");

    expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
    await testPage.setViewportSize({ width: 768, height: 900 });
    await expect(testPage.getByTestId("tasks-list-view")).toBeVisible();
    await expect(testPage.getByTestId("tasks-list-search")).toBeVisible();
    await expect(testPage.getByTestId("tasks-list-archive-modes")).toBeVisible();
    await expect(testPage.getByTestId("mobile-topbar-page-context")).toHaveCount(0);
    await capturePreview(testPage, testInfo, "ui-07-coarse-tablet-768x900");
  } finally {
    await apiClient.updateWorkspace(seedData.workspaceId, {
      ...(previousWorkspace ? { name: previousWorkspace.name } : {}),
    });
    await restoreSidebarViewState(apiClient, seedData.workspaceId, previousViews);
  }
});
