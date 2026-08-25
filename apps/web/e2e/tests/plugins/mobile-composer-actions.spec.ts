/**
 * Pixel 5 coverage for `PluginComposerCapability`
 * (docs/specs/plugins/requirements/voice-extraction-host.md, "Mobile Design Contract").
 *
 * Capability parity, not layout parity: the same action, the same insertion
 * and the same native submit have to work by touch, from the shipped mobile
 * composer toolbar and the shipped full-height dialogs. There is no separate
 * plugin drawer to open.
 *
 * Filename matches /mobile-.*\.spec\.ts/ so the `mobile-chrome` project picks
 * it up.
 */
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import type { ApiClient } from "../../helpers/api-client";
import type { Locator, Page } from "@playwright/test";

const DICTATED = "DICTATED";

async function uninstallViaApi(apiClient: ApiClient): Promise<void> {
  await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
}

function action(scope: Page | Locator): Locator {
  return scope.getByTestId("e2e-composer-action").first();
}

/**
 * What the host owes a plugin action on a phone: a place in the shipped
 * composer toolbar whose hit box is fully on screen and not covered by a
 * native control. The button's own size is the plugin's business — the Voice
 * plugin, for instance, grows its control to 40px on a coarse pointer — and
 * this fixture keeps its buttons deliberately tiny so five of them fit where
 * a real plugin puts one.
 */
async function expectTouchReachable(page: Page, button: Locator): Promise<void> {
  await expect(button).toBeVisible();
  const box = await button.boundingBox();
  expect(box, "the action must have a hit box").not.toBeNull();
  expect(box!.width).toBeGreaterThan(0);
  expect(box!.height).toBeGreaterThan(0);

  const viewport = page.viewportSize();
  expect(viewport).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width);

  // Nothing native may sit on top of it: a tap has to land on the action.
  const hit = await page.evaluate(
    ({ x, y }) => {
      const el = document.elementFromPoint(x, y);
      return el?.closest("[data-testid='e2e-composer-action']") !== null;
    },
    { x: box!.x + box!.width / 2, y: box!.y + box!.height / 2 },
  );
  expect(hit, "a native control is intercepting taps on the plugin action").toBe(true);
}

/**
 * Walks left from the end rather than using Control+Home, which is a no-op
 * under Android emulation and silently leaves the caret at the end. The
 * insertion then appends and the selection-honouring assertion passes for the
 * wrong reason.
 */
async function caretBackFromEnd(target: Locator, steps: number): Promise<void> {
  await target.click();
  for (let i = 0; i < steps; i++) await target.press("ArrowLeft");
}

async function expectNoHorizontalOverflow(page: Page): Promise<void> {
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    await page.evaluate(() => document.documentElement.clientWidth),
  );
}

test.describe("Mobile plugin composer actions", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test.afterEach(async ({ apiClient }) => {
    await uninstallViaApi(apiClient);
  });

  test("task chat: the action is touch-reachable and native submit sends the dictated draft", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await installFixturePlugin(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile composer capability",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const editor = await session.composerReady();

    const composerAction = action(session.activeChat());
    await expect(composerAction).toHaveAttribute("data-surface", "task-chat");
    await expect(composerAction).toHaveAttribute("data-presentation", "mobile");
    await expectTouchReachable(testPage, composerAction.getByTestId("e2e-composer-insert"));

    await editor.fill("head tail");
    await caretBackFromEnd(editor, " tail".length);

    await composerAction.getByTestId("e2e-composer-insert").tap();
    await expect(editor).toHaveText(`head ${DICTATED} tail`);

    await composerAction.getByTestId("e2e-composer-submit").tap();
    await expect(composerAction).toHaveAttribute("data-status", "submitted");
    await session.expectChatResponseVisible(`head ${DICTATED} tail`);

    // The composer toolbar still owns its own row: adding a plugin action
    // must not push the page sideways.
    await expectNoHorizontalOverflow(testPage);
  });

  test("task creation: the action rides the prompt toolbar in the responsive dialog", async ({
    testPage,
  }) => {
    await installFixturePlugin(testPage);

    // The shipped mobile entry point: the global rail is hidden on a phone,
    // so task creation starts from the board's floating action button.
    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();
    await kanban.mobileFab.tap();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    const description = dialog.getByTestId("task-description-input");
    await expect(description).toBeVisible();

    const composerAction = action(dialog);
    await expect(composerAction).toHaveAttribute("data-surface", "task-create");
    await expect(composerAction).toHaveAttribute("data-presentation", "mobile");
    await expectTouchReachable(testPage, composerAction.getByTestId("e2e-composer-insert"));

    await description.fill("head tail");
    await caretBackFromEnd(description, " tail".length);

    await composerAction.getByTestId("e2e-composer-insert").tap();
    await expect(description).toHaveValue(`head ${DICTATED} tail`);

    // The dialog still owns its own scrolling and the page does not pan.
    await expectNoHorizontalOverflow(testPage);
  });

  test("new session: the action is reachable inside the full-height dialog and launches a session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await installFixturePlugin(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile new session composer",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const before = (await apiClient.listTaskSessions(task.id)).sessions.length;

    await session.openMobileNewSessionDialog();
    const dialog = session.newSessionDialog();
    await expect(dialog).toBeVisible();

    const description = dialog.getByTestId("task-description-input");
    await expect(description).toBeVisible();

    const composerAction = action(dialog);
    await expect(composerAction).toHaveAttribute("data-surface", "new-session");
    await expect(composerAction).toHaveAttribute("data-presentation", "mobile");
    await expectTouchReachable(testPage, composerAction.getByTestId("e2e-composer-insert"));

    await description.fill("head tail");
    await caretBackFromEnd(description, " tail".length);

    await composerAction.getByTestId("e2e-composer-insert").tap();
    await expect(description).toHaveValue(`head ${DICTATED} tail`);

    await composerAction.getByTestId("e2e-composer-submit").tap();
    await expect(composerAction).toHaveAttribute("data-status", "submitted");

    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length, {
        timeout: 60_000,
      })
      .toBeGreaterThan(before);
    await expectNoHorizontalOverflow(testPage);
  });
});
