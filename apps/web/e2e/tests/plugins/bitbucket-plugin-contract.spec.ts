import path from "node:path";
import type { Locator, Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const PLUGIN_ID = "kandev-plugin-e2e";
const PACKAGE_PATH = path.resolve(
  __dirname,
  "../../../../../apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz",
);

async function installFixture(page: Page): Promise<void> {
  await page.goto("/settings/plugins");
  await page.getByTestId("install-plugin-trigger").click();
  await page.getByTestId("install-plugin-tab-upload").click();
  await page.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
  await page.getByTestId("install-plugin-upload-submit").click();
  await expect(page.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 15_000 });
}

function visibleEditor(scope: Locator | Page): Locator {
  return scope.locator(".tiptap.ProseMirror:visible").first();
}

async function expectPersistedReference(
  apiClient: ApiClient,
  sessionId: string,
  referenceID: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some(
          (message) =>
            Array.isArray(message.metadata?.entity_references) &&
            message.metadata.entity_references.some(
              (reference) =>
                typeof reference === "object" &&
                reference !== null &&
                (reference as Record<string, unknown>).id === referenceID,
            ),
        );
      },
      { timeout: 15_000, message: `wait for fixture reference ${referenceID}` },
    )
    .toBe(true);
}

test.describe("Bitbucket plugin contract", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("runs the packaged second-provider contract through native host surfaces", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const capture = new PrAssetCapture(testPage, testInfo.file);
    await installFixture(testPage);

    // A declared browser action is authenticated by the host. The fixture
    // exposes Bitbucket-shaped labels but receives only generic context.
    await testPage.goto("/plugins/e2e-hello");
    await testPage.reload();
    await testPage.getByTestId("fixture-connection-status").click();
    await expect(testPage.getByTestId("fixture-connection-result")).toHaveText("Connected");

    // The manifest-declared repository provider participates in the existing
    // native picker and contributes branches plus PR URL inspection.
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await testPage.getByTestId("source-mode-remote").click();
    await testPage.getByTestId("remote-repo-chip-trigger").first().click();
    await testPage.getByTestId("remote-repo-option").filter({ hasText: "TEAM/fixture" }).click();
    await expect(testPage.getByTestId("remote-repo-chip-trigger").first()).toContainText(
      "TEAM/fixture",
    );
    await expect(testPage.getByTestId("remote-branch-chip-trigger")).toContainText("main");
    await testPage.getByRole("button", { name: "Cancel" }).click();

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Bitbucket plugin contract task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("fixture contract task has no session");

    // Native Link submenu action invokes the declared task-scoped action.
    await kanban.goto();
    await kanban.openTaskContextMenu(task.id);
    const linkSubmenu = testPage.getByTestId("task-context-link");
    await linkSubmenu.focus();
    await testPage.keyboard.press("ArrowRight");
    const bitbucketLink = testPage.getByTestId(
      "task-context-link-plugin-kandev-plugin-e2e:link-bitbucket-pull-request",
    );
    await expect(bitbucketLink).toHaveText("Bitbucket Pull Request");
    await expect(bitbucketLink.getByTestId("fixture-bitbucket-icon")).toBeVisible();
    await bitbucketLink.click();
    const linkDialog = testPage.getByRole("dialog", { name: "Link Bitbucket pull request" });
    await expect(linkDialog).toContainText("Use a Bitbucket pull request URL for this task.");
    await linkDialog.getByRole("button", { name: "Save" }).click();
    await expect(linkDialog.getByTestId("fixture-link-pull-request-error")).toHaveText(
      "Enter a Bitbucket pull request URL.",
    );
    await linkDialog
      .getByLabel("Pull request")
      .fill("https://bitbucket.example.test/projects/TEAM/repos/fixture/pull-requests/42");
    await linkDialog.getByRole("button", { name: "Save" }).click();
    await expect(linkDialog).toBeHidden();
    await expect(
      testPage.getByText("Bitbucket pull request linked", { exact: true }),
    ).toBeVisible();

    // Desktop sessions expose provider reviews as Dockview panels. The mobile
    // companion covers the shared multi-review selector.
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    // Desktop sessions expose provider reviews as Dockview tabs, rather than
    // the center-layout's semantic Review tab used on smaller screens.
    await testPage.getByText("Bitbucket Pull Request #42", { exact: true }).click();
    await expect(testPage.getByTestId("fixture-review-panel-desktop")).toHaveAttribute(
      "data-review-key",
      "pull-request-42",
    );
    await capture.screenshot("desktop-native-review", {
      caption: "Desktop native review panel supplied by a source-control plugin",
    });
    capture.flush();

    // A candidate that is revoked after search must fail at send time and the
    // error must not disclose the fixture's credential value. Exercise this
    // first because entity-source menus dispose their selected source after a
    // successful submission.
    await session.clickSessionChatTab();
    await session.waitForChatIdle({ timeout: 30_000 });
    const editor = visibleEditor(session.activeChat());
    await editor.fill("");
    await editor.pressSequentially("#revoked");
    await testPage.getByRole("option", { name: /Pull request #99/ }).click();
    await expect(session.activeChat().getByTestId("entity-reference-chip")).toBeVisible();
    await Promise.all([
      expect(testPage.getByText("Message send status unknown", { exact: true })).toBeVisible(),
      session.activeChat().getByTestId("submit-message-button").click(),
    ]);
    await expect(testPage.locator("body")).not.toContainText("fixture-credential-secret");

    // Search returns a manifest-owned source; a selected good reference is
    // reauthorized by the live plugin during submission before metadata persists.
    await editor.fill("");
    await editor.pressSequentially("#Provider-neutral");
    await testPage.getByRole("option", { name: /Pull request #42/ }).click();
    await expect(session.activeChat().getByTestId("entity-reference-chip")).toBeVisible();
    await session.activeChat().getByTestId("submit-message-button").click();
    await expectPersistedReference(apiClient, task.session_id, "pull-request-42");

    // A plugin-owned watch action creates a host task; the host stamps its
    // provenance, while the fixture only requests a generic task write.
    const watch = await apiClient.rawRequest(
      "POST",
      `/api/plugins/${PLUGIN_ID}/actions/watch-create-task`,
      { workspaceId: seedData.workspaceId },
    );
    expect(watch.ok).toBe(true);
    const watchBody = (await watch.json()) as { task_id?: string; watch_created?: boolean };
    expect(watchBody.watch_created).toBe(true);
    expect(watchBody.task_id).toBeTruthy();
    const watchedTask = await apiClient.rawRequest("GET", `/api/v1/tasks/${watchBody.task_id}`);
    expect(watchedTask.ok).toBe(true);
    expect((await watchedTask.json()) as { metadata?: { source?: string } }).toMatchObject({
      metadata: { source: `plugin:${PLUGIN_ID}` },
    });

    // Disable/re-enable must revoke all native registrations then restore them
    // from the package without leaving a stale provider or review panel.
    await testPage.goto("/settings/plugins");
    const row = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await row.getByRole("button", { name: "Disable" }).click();
    await expect(row.getByText("Disabled", { exact: true })).toBeVisible();
    await testPage.goto(`/t/${task.id}`);
    await expect(testPage.getByTestId("fixture-review-panel-desktop")).toHaveCount(0);
    await testPage.goto("/settings/plugins");
    await row.getByRole("button", { name: "Enable" }).click();
    await expect(row.getByText("Active", { exact: true })).toBeVisible();
    await testPage.goto(`/t/${task.id}`);
    // Re-enabling restores the provider registration, but must not override
    // the native per-session rule that a previously offered review panel stays
    // dismissed until the user opens it again.
    await testPage.getByText("Bitbucket Pull Request #42", { exact: true }).click();
    await expect(testPage.getByTestId("fixture-review-panel-desktop")).toBeVisible();
  });
});
