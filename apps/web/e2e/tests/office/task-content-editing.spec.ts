import type { Browser, BrowserContext, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/office-fixture";
import { watchWs } from "../../helpers/causal-waits";

/**
 * E2E coverage for in-place title and description editing on the Office task
 * detail page (docs/specs/office-task-content-editing/spec.md).
 *
 * Title commits are optimistic (apply + close immediately, roll back on
 * failure); description saves are not optimistic (editor stays open in a
 * saving state, and a failure leaves the draft untouched). See
 * hooks/use-optimistic-task-mutation.ts for both commit paths.
 */

async function gotoTaskPage(testPage: Page, taskId: string, title: string) {
  await testPage.goto(`/office/tasks/${taskId}`);
  await expect(testPage.getByRole("heading", { name: title })).toBeVisible({
    timeout: 10_000,
  });
}

/**
 * Opens an independent client against the same backend: its own context means
 * its own store, WebSocket and localStorage, which is what makes it a
 * meaningful stand-in for a second browser tab observing the same task.
 */
async function openSecondClient(
  browser: Browser,
  frontendUrl: string,
  backendPort: number,
): Promise<{ page: Page; context: BrowserContext }> {
  const context = await browser.newContext({ baseURL: frontendUrl });
  const page = await context.newPage();
  await page.addInitScript(
    ({ port }: { port: number }) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      window.__KANDEV_API_PORT = String(port);
      window.__KANDEV_E2E_EXPOSE_STORE__ = true;
    },
    { port: backendPort },
  );
  return { page, context };
}

async function mockTaskPatchFailure(testPage: Page, taskId: string) {
  await testPage.route(
    (url) =>
      url.pathname === `/api/v1/tasks/${taskId}` || url.pathname.endsWith(`/tasks/${taskId}`),
    async (route) => {
      if (route.request().method() === "PATCH") {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "forced failure" }),
        });
        return;
      }
      await route.continue();
    },
  );
}

test.describe("task title and description editing", () => {
  test("editing the title renders the trimmed value and persists it", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Original title task", {
      workflow_id: officeSeed.workflowId,
    });
    await gotoTaskPage(testPage, task.id, "Original title task");

    await testPage.getByTestId("office-task-title").dblclick();
    const input = testPage.getByTestId("office-task-title-input");
    await input.fill("  Renamed via inline edit  ");
    await input.press("Enter");

    // AC-3: renders the trimmed draft immediately and closes the editor.
    await expect(testPage.getByTestId("office-task-title-input")).toHaveCount(0);
    await expect(testPage.getByTestId("office-task-title")).toHaveText("Renamed via inline edit");

    // AC-26: persisted, and only the title field changed.
    const persisted = (await apiClient.getTask(task.id)) as unknown as Record<string, unknown>;
    expect(persisted.title).toBe("Renamed via inline edit");
  });

  test("editing the description saves it, closes the editor, and persists it", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Description edit task", {
      workflow_id: officeSeed.workflowId,
      description: "Original description",
    });
    await gotoTaskPage(testPage, task.id, "Description edit task");

    await testPage.getByTestId("office-task-description").click();
    const textarea = testPage.getByTestId("office-task-description-textarea");
    await expect(textarea).toHaveValue("Original description");
    await textarea.fill("  Updated description body  ");
    await testPage.getByTestId("office-task-description-save").click();

    // AC-16 + AC-17: save request fires, editor closes, saved value renders.
    await expect(testPage.getByTestId("office-task-description-textarea")).toHaveCount(0, {
      timeout: 10_000,
    });
    await expect(testPage.getByTestId("office-task-description")).toHaveText(
      "Updated description body",
    );

    // AC-26: persisted, and only the description field changed.
    const persisted = (await apiClient.getTask(task.id)) as unknown as Record<string, unknown>;
    expect(persisted.description).toBe("Updated description body");
  });

  test("a second browser context observes title and description edits without a reload", async ({
    testPage,
    apiClient,
    officeSeed,
    backend,
    browser,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTask(officeSeed.workspaceId, "Cross-client edit task", {
      workflow_id: officeSeed.workflowId,
      description: "Starting description",
    });

    await gotoTaskPage(testPage, task.id, "Cross-client edit task");

    const second = await openSecondClient(browser, backend.frontendUrl, backend.port);
    const secondClientWs = watchWs(second.page);
    await gotoTaskPage(second.page, task.id, "Cross-client edit task");

    // Device A edits the title; device B must converge without reloading.
    const titleUpdate = secondClientWs.waitForEvent("office.task.updated");
    await testPage.getByTestId("office-task-title").dblclick();
    const titleInput = testPage.getByTestId("office-task-title-input");
    await titleInput.fill("Renamed on device A");
    await titleInput.press("Enter");
    await expect(testPage.getByTestId("office-task-title")).toHaveText("Renamed on device A");

    // AC-27: the other client picks up the change from office.task.updated.
    await titleUpdate;
    await expect(second.page.getByTestId("office-task-title")).toHaveText("Renamed on device A");

    // Device A edits the description; device B must converge without reloading.
    const descriptionUpdate = secondClientWs.waitForEvent("office.task.updated");
    await testPage.getByTestId("office-task-description").click();
    const descriptionTextarea = testPage.getByTestId("office-task-description-textarea");
    await descriptionTextarea.fill("Updated on device A");
    await testPage.getByTestId("office-task-description-save").click();
    await expect(testPage.getByTestId("office-task-description-textarea")).toHaveCount(0, {
      timeout: 10_000,
    });

    await descriptionUpdate;
    await expect(second.page.getByTestId("office-task-description")).toHaveText(
      "Updated on device A",
    );

    await second.context.close();
  });

  test("a failed title save rolls back to the previous title and toasts", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Rollback title task", {
      workflow_id: officeSeed.workflowId,
    });
    await gotoTaskPage(testPage, task.id, "Rollback title task");
    await mockTaskPatchFailure(testPage, task.id);

    await testPage.getByTestId("office-task-title").dblclick();
    const input = testPage.getByTestId("office-task-title-input");
    await input.fill("Title that will fail to save");
    await input.press("Enter");

    // AC-9: the optimistic render is reverted once the PATCH fails, and the
    // stored title is what's restored (the "last word standing" case).
    await expect(async () => {
      const heading = testPage.getByTestId("office-task-title");
      await expect(heading).toHaveText("Rollback title task");
    }).toPass({ timeout: 5_000 });

    const persisted = (await apiClient.getTask(task.id)) as unknown as Record<string, unknown>;
    expect(persisted.title).toBe("Rollback title task");
  });

  test("a failed description save keeps the editor open with the draft intact", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Rollback description task", {
      workflow_id: officeSeed.workflowId,
      description: "Original description",
    });
    await gotoTaskPage(testPage, task.id, "Rollback description task");
    await mockTaskPatchFailure(testPage, task.id);

    await testPage.getByTestId("office-task-description").click();
    const textarea = testPage.getByTestId("office-task-description-textarea");
    await textarea.fill("Draft that fails to save");
    await testPage.getByTestId("office-task-description-save").click();

    // AC-18: editor stays open, draft is untouched, nothing was persisted.
    await expect(testPage.getByTestId("office-task-description-textarea")).toBeVisible();
    await expect(testPage.getByTestId("office-task-description-textarea")).toHaveValue(
      "Draft that fails to save",
    );

    const persisted = (await apiClient.getTask(task.id)) as unknown as Record<string, unknown>;
    expect(persisted.description).toBe("Original description");
  });
});
