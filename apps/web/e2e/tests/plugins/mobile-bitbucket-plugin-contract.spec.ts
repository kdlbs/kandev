import path from "node:path";
import type { Locator, Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";

const PLUGIN_ID = "kandev-plugin-e2e";
const PACKAGE_PATH = path.resolve(
  __dirname,
  "../../../../../apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz",
);

async function installFixture(page: Page): Promise<void> {
  await page.goto("/settings/plugins");
  await page.getByTestId("install-plugin-trigger").tap();
  await page.getByTestId("install-plugin-tab-upload").tap();
  await page.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
  await page.getByTestId("install-plugin-upload-submit").tap();
  await expect(page.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 15_000 });
}

function visibleEditor(scope: Locator | Page): Locator {
  return scope.locator(".tiptap.ProseMirror:visible").first();
}

test.describe("mobile Bitbucket plugin contract", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("keeps provider picker, Link action, review selector, and composer source touch-usable", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const capture = new PrAssetCapture(testPage, testInfo.file);
    await installFixture(testPage);

    // The plugin provider is a first-class option in the phone's native
    // Remote picker, with no desktop-only hover path or horizontal overflow.
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.tap();
    await testPage.getByTestId("source-mode-remote").tap();
    await testPage.getByTestId("remote-repo-chip-trigger").first().tap();
    const repositoryOption = testPage
      .getByTestId("remote-repo-option")
      .filter({ hasText: "TEAM/fixture" });
    await expect(repositoryOption).toBeVisible();
    await repositoryOption.tap();
    await expect(testPage.getByTestId("remote-repo-chip-trigger").first()).toContainText(
      "TEAM/fixture",
    );
    await expect(testPage.getByTestId("remote-branch-chip-trigger")).toContainText("main");
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
    await testPage.getByRole("button", { name: "Cancel" }).tap();

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Bitbucket plugin contract task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("mobile fixture contract task has no session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").tap();
    const taskDrawer = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = taskDrawer
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile Bitbucket plugin contract task" });
    await taskRow.getByRole("button", { name: "Task actions" }).tap();
    const linkSubmenu = testPage.getByRole("menuitem", { name: "Link", exact: true });
    await linkSubmenu.tap();
    const bitbucketLink = testPage.getByTestId(
      "task-context-link-plugin-kandev-plugin-e2e:link-bitbucket-pull-request",
    );
    await expect(bitbucketLink).toHaveText("Bitbucket Pull Request");
    await expect(bitbucketLink.getByTestId("fixture-bitbucket-icon")).toBeVisible();
    await bitbucketLink.tap();
    const linkDialog = testPage.getByRole("dialog", { name: "Link Bitbucket pull request" });
    await expect(linkDialog).toHaveAttribute("data-slot", "dialog-content");
    await linkDialog
      .getByLabel("Pull request")
      .fill("https://bitbucket.example.test/projects/TEAM/repos/fixture/pull-requests/42");
    await linkDialog.getByRole("button", { name: "Save" }).tap();
    await expect(linkDialog).toBeHidden();
    await expect(
      testPage.getByText("Bitbucket pull request linked", { exact: true }),
    ).toBeVisible();

    // The responsive task layout presents the exact same plugin reviews via
    // the mobile bottom navigation and its 44px selector menu rows.
    await testPage.getByRole("button", { name: "Review", exact: true }).tap();
    await expect(testPage.getByTestId("mobile-review-panel")).toBeVisible();
    const selector = testPage.getByTestId("review-item-selector-trigger");
    expect((await selector.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await selector.tap();
    const reviewOption = testPage.getByTestId(
      "review-item-selector-item-fixture-source-control:pull-request-42",
    );
    expect((await reviewOption.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await reviewOption.tap();
    await expect(testPage.getByTestId("fixture-review-panel-mobile")).toHaveAttribute(
      "data-review-key",
      "pull-request-42",
    );
    await expect(reviewOption).not.toBeVisible();
    await capture.screenshot("mobile-native-review", {
      caption: "Mobile native review panel supplied by the same source-control plugin",
    });
    capture.flush();

    // A touch-selected composer result still travels through submit-time
    // authorization before the message may be persisted.
    await testPage.getByRole("button", { name: "Chat", exact: true }).tap();
    await session.waitForChatIdle({ timeout: 30_000 });
    const editor = visibleEditor(session.activeChat());
    await editor.tap();
    await editor.fill("");
    await editor.pressSequentially("#Provider-neutral");
    const sourceOption = testPage.getByRole("option", { name: /Pull request #42/ });
    await sourceOption.tap();
    await session.activeChat().getByTestId("submit-message-button").tap();
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(task.session_id!);
          return messages.some(
            (message) =>
              Array.isArray(message.metadata?.entity_references) &&
              message.metadata.entity_references.some(
                (reference) =>
                  typeof reference === "object" &&
                  reference !== null &&
                  (reference as Record<string, unknown>).id === "pull-request-42",
              ),
          );
        },
        { timeout: 15_000, message: "wait for fixture reference pull-request-42" },
      )
      .toBe(true);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
  });
});
