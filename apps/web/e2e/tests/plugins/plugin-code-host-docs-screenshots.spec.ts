/**
 * Focused plugin-authoring docs captures. This generator is skipped in CI.
 * It installs the packaged Bitbucket plugin into an isolated E2E backend so
 * every image exercises the public plugin contract and host-owned UI.
 *
 * From apps/web, set CAPTURE_DOCS_MEDIA=1, DOCS_CODE_HOST_PLUGIN_PACKAGE to
 * the packaged plugin, and DOCS_SCREENSHOTS_DIR to a staging directory; then
 * run this spec through `pnpm e2e:run --host --project chromium -- ...`.
 */
import fs from "node:fs";
import path from "node:path";
import type { Locator, Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import {
  CODE_HOST_PULL_REQUEST as PULL_REQUEST,
  CODE_HOST_REVIEW_KEY as REVIEW_KEY,
} from "./plugin-code-host-docs-fixture";

const CAPTURE = process.env.CAPTURE_DOCS_MEDIA === "1";
const PLUGIN_ID = "kandev-plugin-bitbucket";
const PACKAGE_PATH = process.env.DOCS_CODE_HOST_PLUGIN_PACKAGE?.trim();
const FIXTURE_ID = "kandev-plugin-e2e";
const FIXTURE_PACKAGE = path.resolve(
  __dirname,
  "../../../../../apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz",
);
const SCREENSHOTS_DIR = process.env.DOCS_SCREENSHOTS_DIR
  ? path.resolve(process.env.DOCS_SCREENSHOTS_DIR)
  : path.resolve(__dirname, "../../../../../docs/screenshots");

async function installPlugin(page: Page, id: string, packagePath: string): Promise<void> {
  await page.goto("/settings/plugins");
  await page.getByTestId("install-plugin-trigger").click();
  await page.getByTestId("install-plugin-tab-upload").click();
  await page.getByTestId("install-plugin-file-input").setInputFiles(packagePath);
  await page.getByTestId("install-plugin-upload-submit").click();
  const row = page.getByTestId(`plugin-row-${id}`);
  await expect(row).toBeVisible({ timeout: 15_000 });
  await expect(row.getByText("Active", { exact: true })).toBeVisible();
}

async function settle(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await document.fonts.ready;
    const finite = document.getAnimations().filter((animation) => {
      const iterations = animation.effect?.getComputedTiming().iterations;
      return typeof iterations === "number" && Number.isFinite(iterations);
    });
    await Promise.all(finite.map((animation) => animation.finished.catch(() => undefined)));
  });
}

async function focusedShot(
  page: Page,
  name: string,
  locators: Locator[],
  padding = 16,
  bottomPadding = padding,
): Promise<void> {
  await settle(page);
  fs.mkdirSync(SCREENSHOTS_DIR, { recursive: true });
  const boxes = [];
  for (const locator of locators) {
    for (let index = 0; index < (await locator.count()); index += 1) {
      const item = locator.nth(index);
      if (await item.isVisible()) {
        const box = await item.boundingBox();
        if (box) boxes.push(box);
      }
    }
  }
  if (boxes.length === 0) throw new Error(`No visible capture target for ${name}`);
  const viewport = page.viewportSize();
  if (!viewport) throw new Error("Viewport unavailable");
  const left = Math.max(0, Math.min(...boxes.map((box) => box.x)) - padding);
  const top = Math.max(0, Math.min(...boxes.map((box) => box.y)) - padding);
  const right = Math.min(
    viewport.width,
    Math.max(...boxes.map((box) => box.x + box.width)) + padding,
  );
  const bottom = Math.min(
    viewport.height,
    Math.max(...boxes.map((box) => box.y + box.height)) + bottomPadding,
  );
  await page.screenshot({
    path: path.join(SCREENSHOTS_DIR, `plugin-code-host-${name}.png`),
    clip: { x: left, y: top, width: right - left, height: bottom - top },
  });
}

async function mockBitbucket(page: Page, taskId: string): Promise<void> {
  const repositories = {
    repositories: [
      {
        provider_id: "bitbucket",
        provider_host: "bitbucket.org",
        owner_or_project: "northstar-labs",
        provider_repository_id: "northstar-labs/relay",
        name: "relay",
        clone_url: "https://bitbucket.org/northstar-labs/relay.git",
        default_branch: "main",
      },
    ],
  };
  const queue = { pull_requests: [PULL_REQUEST] };
  await page.route(`**/api/plugins/${PLUGIN_ID}/actions/**`, async (route) => {
    const action = new URL(route.request().url()).pathname.split("/").at(-1) ?? "";
    const request = route.request().postDataJSON() as { body?: { view?: string } } | null;
    const responses: Record<string, unknown> = {
      "connection.get": { state: "connected", healthy: true, product: "cloud" },
      "repositories.list": repositories,
      "pullrequests.queue": queue,
      "pullrequests.search": queue,
      "pullrequests.associations": {
        associations: [
          {
            review_key: REVIEW_KEY,
            task_id: taskId,
            task_title: "Audit export retention controls",
          },
        ],
      },
      "watches.get": { watches: [] },
    };
    let response = responses[action];
    if (action === "pullrequests.get") {
      response = request?.body?.view === "task" ? queue : PULL_REQUEST;
    }
    if (response === undefined) return route.continue();
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(response),
    });
  });
}

test.describe("Plugin code-host docs screenshots", () => {
  test.skip(!CAPTURE || !PACKAGE_PATH, "set CAPTURE_DOCS_MEDIA and DOCS_CODE_HOST_PLUGIN_PACKAGE");

  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
    await apiClient.rawRequest("DELETE", `/api/plugins/${FIXTURE_ID}`).catch(() => undefined);
  });

  test("captures host-owned code-host surfaces", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(150_000);
    await testPage.setViewportSize({ width: 1440, height: 900 });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Audit export retention controls",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id || !PACKAGE_PATH) throw new Error("Capture task or package unavailable");
    await mockBitbucket(testPage, task.id);
    await installPlugin(testPage, PLUGIN_ID, PACKAGE_PATH);

    await testPage.goto("/bitbucket");
    const workbench = testPage.getByTestId("bitbucket-workbench");
    await expect(workbench.getByText(PULL_REQUEST.title)).toBeVisible();
    await focusedShot(testPage, "dashboard", [
      workbench.getByTestId("bitbucket-scope-bar"),
      workbench.getByTestId("bitbucket-list-toolbar"),
      workbench.getByTestId("bitbucket-pr-queue"),
    ]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await testPage.getByTestId("source-mode-remote").click();
    await testPage.getByTestId("remote-repo-chip-trigger").first().click();
    const repository = testPage
      .getByTestId("remote-repo-option")
      .filter({ hasText: "northstar-labs/relay" });
    await expect(repository).toBeVisible();
    await focusedShot(testPage, "repository-picker", [testPage.getByRole("dialog"), repository]);
    await testPage.getByRole("button", { name: "Cancel" }).click();

    await kanban.openTaskContextMenu(task.id);
    const link = testPage.getByTestId("task-context-link");
    await link.focus();
    await testPage.keyboard.press("ArrowRight");
    const bitbucketLink = testPage.getByTestId(
      "task-context-link-plugin-kandev-plugin-bitbucket:link-pull-request",
    );
    await expect(bitbucketLink).toBeVisible();
    await focusedShot(testPage, "link-menu", [testPage.locator('[role="menu"]:visible')]);
    await bitbucketLink.click();
    const linkDialog = testPage.getByRole("dialog", { name: "Link Bitbucket pull request" });
    await expect(linkDialog).toBeVisible();
    await focusedShot(testPage, "link-dialog", [linkDialog]);
    await testPage.keyboard.press("Escape");

    await testPage.goto("/");
    const taskRow = testPage
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Audit export retention controls" });
    const icon = taskRow.getByTestId(`registered-change-request-task-icon-${task.id}`);
    await icon.hover();
    const summary = testPage.locator('[data-testid="pr-task-status-summary"]:visible').first();
    await expect(summary).toBeVisible();
    await expect(icon).toHaveClass(/text-sky-400/);
    await focusedShot(testPage, "sidebar-status", [taskRow, summary]);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const chip = testPage.getByTestId("integration-change-request-status-chip").first();
    await expect(chip).toBeVisible();
    await chip.hover();
    const statusPopover = testPage.getByTestId("integration-change-request-status-popover");
    await expect(statusPopover).toBeVisible();
    await focusedShot(testPage, "ci-popover", [chip, statusPopover]);
    await testPage.keyboard.press("Escape");

    const topbar = testPage.getByTestId("integration-change-request-status-trigger").first();
    await topbar.click();
    const review = testPage.getByTestId("change-request-detail");
    await expect(review).toBeVisible();
    await focusedShot(testPage, "review-panel", [review]);
  });

  test("captures dynamic composer reference search", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 1100, height: 760 });
    await installPlugin(testPage, FIXTURE_ID, FIXTURE_PACKAGE);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Provider-neutral reference search",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("Reference capture task has no session");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const editor = session.activeChat().locator(".tiptap.ProseMirror:visible").first();
    await editor.fill("");
    await editor.pressSequentially("#Provider-neutral");
    const option = testPage.getByRole("option", { name: /Pull request #42/ });
    await expect(option).toBeVisible();
    await focusedShot(testPage, "reference-search", [editor, option], 32, 4);
  });
});
