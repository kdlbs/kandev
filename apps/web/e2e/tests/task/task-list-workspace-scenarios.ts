import { test, expect } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

async function navigateInSpa(page: Page, href: string) {
  await page.evaluate((url) => {
    window.history.pushState({}, "", url);
    window.dispatchEvent(new PopStateEvent("popstate"));
  }, href);
}

async function activate(control: Locator, touch: boolean) {
  if (touch) {
    expect((await control.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await control.tap();
  } else await control.click();
}

export function taskListWorkspaceScenarios(touch: boolean) {
  taskListPartialMetadataScenario();
  taskListWorkspaceHistoryScenario();
  // @covers AC-TASKS-LIST-ARCHIVE-FILTERS-004.5
  test("recovers a failed workspace lookup and offers Home for an unknown workspace", async ({
    testPage,
    seedData,
  }) => {
    await testPage.goto("/settings");
    let fail = true;
    await testPage.route("**/api/v1/workspaces", async (route) => {
      if (fail)
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "private workspace diagnostic" }),
        });
      else await route.continue();
    });
    await navigateInSpa(
      testPage,
      `/tasks?workspace=${seedData.workspaceId}&archive=archived&workflow=all&repository=all`,
    );
    await expect(testPage.getByRole("alert")).toHaveText("Failed to load route");
    await expect(testPage.getByText("private workspace diagnostic")).toHaveCount(0);
    fail = false;
    await activate(testPage.getByRole("button", { name: "Retry", exact: true }), touch);
    await expect(testPage.getByText("No archived tasks", { exact: true })).toBeVisible();
    await navigateInSpa(testPage, "/tasks?workspace=missing&archive=archived");
    await expect(testPage.getByRole("alert")).toHaveText("Workspace not found");
    await expect(testPage.getByTestId("tasks-list")).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage);
    await activate(testPage.getByRole("button", { name: "Home", exact: true }), touch);
    await expect(testPage).toHaveURL(`/tasks?workspace=${seedData.workspaceId}`);
    await expect(testPage.getByTestId("tasks-list-archive-active")).toHaveAttribute(
      "data-state",
      "on",
    );
    await expect(testPage.getByRole("alert")).toHaveCount(0);
  });
}

function taskListWorkspaceHistoryScenario() {
  // @covers AC-TASKS-LIST-ARCHIVE-FILTERS-004.2 AC-TASKS-LIST-ARCHIVE-FILTERS-004.3
  test("restores each linked workspace and its filters through browser history", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const other = await apiClient.createWorkspace("Portable archive workspace");
    let releaseMetadata: (() => void) | undefined;
    try {
      const workflow = await apiClient.createWorkflow(other.id, "Portable workflow", "simple");
      const { steps } = await apiClient.listWorkflowSteps(workflow.id);
      const fixtures = [
        [seedData.workspaceId, seedData.workflowId, seedData.startStepId, "Portable archive A"],
        [other.id, workflow.id, steps[0].id, "Portable archive B"],
      ];
      const links: string[] = [];
      for (const [workspaceId, workflowId, stepId, title] of fixtures) {
        const task = await apiClient.createTask(workspaceId, title, {
          workflow_id: workflowId,
          workflow_step_id: stepId,
        });
        await apiClient.archiveTask(task.id);
        links.push(
          `/tasks?workspace=${workspaceId}&archive=archived&q=Portable&workflow=${workflowId}&repository=all&sort=title_asc&group=none`,
        );
      }
      await testPage.goto("/settings");
      const matchesMetadata = (url: URL) =>
        url.pathname === "/api/v1/workflows" && url.searchParams.get("workspace_id") === other.id;
      const metadataReady = new Promise<void>((resolve) => {
        releaseMetadata = resolve;
      });
      await testPage.route(matchesMetadata, async (route) => {
        await metadataReady;
        await route.continue();
      });
      for (let index = 0; index < links.length; index++) {
        const requested =
          index === 1
            ? testPage.waitForRequest((request) => matchesMetadata(new URL(request.url())))
            : null;
        await navigateInSpa(testPage, links[index]);
        if (index === 1) {
          await requested;
          await expect(testPage.getByText("Loading Tasks…", { exact: true })).toBeVisible();
          await expect(testPage.getByTestId("tasks-list-workflow-filter")).toHaveCount(0);
          releaseMetadata?.();
        }
        await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText([fixtures[index][3]]);
        expect(new URL(testPage.url()).search).toBe(new URL(links[index], testPage.url()).search);
      }
      await testPage.goBack();
      await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText([fixtures[0][3]]);
      await testPage.goForward();
      await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText([fixtures[1][3]]);
      await expect(testPage.getByTestId("tasks-list-search")).toHaveValue("Portable");
      await assertNoDocumentHorizontalOverflow(testPage);
    } finally {
      releaseMetadata?.();
      await testPage.goto("/settings");
      await apiClient.deleteWorkspace(other.id, other.name);
    }
  });
}

function taskListPartialMetadataScenario() {
  // @covers AC-TASKS-LIST-ARCHIVE-FILTERS-004.5
  test("loads a workflow when unused repository metadata fails", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const archived = await apiClient.createTask(seedData.workspaceId, "Partial metadata archive", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.archiveTask(archived.id);
    await testPage.goto("/settings");
    await testPage.route("**/api/v1/workspaces/*/repositories", (route) =>
      route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "Repository metadata unavailable" }),
      }),
    );
    await navigateInSpa(
      testPage,
      `/tasks?workspace=${seedData.workspaceId}&archive=archived&workflow=${seedData.workflowId}&repository=all`,
    );
    await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText([
      "Partial metadata archive",
    ]);
    await expect(testPage.getByTestId("tasks-list-error")).toHaveCount(0);
  });
}
