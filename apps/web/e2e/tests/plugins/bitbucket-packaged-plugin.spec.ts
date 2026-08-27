import path from "node:path";
import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";

const PLUGIN_ID = "kandev-plugin-bitbucket";
const packagePath = process.env.KANDEV_BITBUCKET_PLUGIN_PACKAGE?.trim();
const captureEnabled = Boolean(process.env.CAPTURE_PR_ASSETS);

const CAPTURE_ACTION_RESPONSES: Record<string, unknown> = {
  "connection.get": { state: "connected", healthy: true, product: "cloud" },
  "repositories.list": {
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
      {
        provider_id: "bitbucket",
        provider_host: "bitbucket.org",
        owner_or_project: "northstar-labs",
        provider_repository_id: "northstar-labs/console",
        name: "console",
        clone_url: "https://bitbucket.org/northstar-labs/console.git",
        default_branch: "main",
      },
    ],
  },
  "pullrequests.queue": {
    pull_requests: [
      {
        id: "42",
        number: 42,
        title: "Add audit log export with retention controls",
        url: "https://bitbucket.org/northstar-labs/relay/pull-requests/42",
        repository_id: "northstar-labs/relay",
        repository_name: "relay",
        state: "OPEN",
        author_display_name: "Maya Chen",
        created_at: "2026-08-07T09:30:00Z",
        source_branch: "feature/audit-export",
        destination_branch: "main",
        status: { label: "Needs review", state: "pending" },
        capabilities: ["launch_task"],
      },
      {
        id: "37",
        number: 37,
        title: "Fix pipeline cache invalidation",
        url: "https://bitbucket.org/northstar-labs/console/pull-requests/37",
        repository_id: "northstar-labs/console",
        repository_name: "console",
        state: "OPEN",
        author_display_name: "Noah Williams",
        created_at: "2026-08-06T16:15:00Z",
        source_branch: "fix/pipeline-cache",
        destination_branch: "main",
        status: { label: "Checks failing", state: "failed" },
        capabilities: ["launch_task"],
      },
      {
        id: "31",
        number: 31,
        title: "Harden OAuth callback state validation",
        url: "https://bitbucket.org/northstar-labs/relay/pull-requests/31",
        repository_id: "northstar-labs/relay",
        repository_name: "relay",
        state: "OPEN",
        author_display_name: "Ari Almeida",
        created_at: "2026-08-05T11:00:00Z",
        source_branch: "security/oauth-state",
        destination_branch: "main",
        status: { label: "Approved", state: "approved" },
        capabilities: ["launch_task"],
      },
    ],
  },
  "pullrequests.associations": { associations: [] },
};

const LINKED_REVIEW_KEY = "northstar-labs/relay#42";

function linkedPullRequest(taskId: string) {
  return {
    id: "42",
    review_key: LINKED_REVIEW_KEY,
    number: 42,
    title: "Add audit log export with retention controls",
    url: "https://bitbucket.org/northstar-labs/relay/pull-requests/42",
    provider_scope: "https://bitbucket.org",
    repository_id: "northstar-labs/relay",
    repository_name: "relay",
    state: "OPEN",
    source_branch: "feature/audit-export",
    destination_branch: "main",
    updated_at: "2026-08-10T15:30:00Z",
    associations: [{ review_key: LINKED_REVIEW_KEY, task_id: taskId }],
    participants: [{ id: "reviewer-1", name: "Maya Chen", role: "REVIEWER", approved: true }],
    statuses: [
      {
        key: "build",
        name: "Build",
        state: "SUCCESSFUL",
        url: "https://bitbucket.org/northstar-labs/relay/addon/pipelines/home#!/results/84",
      },
    ],
  };
}

async function mockLinkedPullRequestState(page: Page, taskId: string): Promise<() => number> {
  let detailRequests = 0;
  const pullRequest = linkedPullRequest(taskId);
  await page.route(`**/api/plugins/${PLUGIN_ID}/actions/**`, async (route) => {
    const actionKey = new URL(route.request().url()).pathname.split("/").at(-1) ?? "";
    if (actionKey === "pullrequests.associations") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          associations: [
            {
              review_key: LINKED_REVIEW_KEY,
              provider_scope: "https://bitbucket.org",
              repository_id: "northstar-labs/relay",
              number: 42,
              task_id: taskId,
              task_title: "Sidebar summary",
            },
          ],
        }),
      });
      return;
    }
    if (actionKey === "pullrequests.get") {
      detailRequests += 1;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ...pullRequest, pull_requests: [pullRequest] }),
      });
      return;
    }
    await route.continue();
  });
  return () => detailRequests;
}

test.skip(!packagePath, "requires KANDEV_BITBUCKET_PLUGIN_PACKAGE from the attached plugin repo");

async function mockConfiguredCaptureState(page: Page): Promise<void> {
  if (!captureEnabled) return;
  await page.route(`**/api/plugins/${PLUGIN_ID}/actions/**`, async (route) => {
    const actionKey = new URL(route.request().url()).pathname.split("/").at(-1) ?? "";
    const response = CAPTURE_ACTION_RESPONSES[actionKey];
    if (response === undefined) return route.continue();
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(response),
    });
  });
}

async function expectNoHorizontalOverflow(page: Page): Promise<void> {
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    )
    .toBe(true);
}

async function expectContainedInViewport(page: Page, selector: string): Promise<void> {
  const element = page.locator(selector);
  await expect
    .poll(async () => {
      const [box, viewport] = await Promise.all([element.boundingBox(), page.viewportSize()]);
      if (!box || !viewport) return false;
      return (
        box.x >= -1 &&
        box.y >= -1 &&
        box.x + box.width <= viewport.width + 1 &&
        box.y + box.height <= viewport.height + 1
      );
    })
    .toBe(true);
}

async function installPackagedPlugin(testPage: import("@playwright/test").Page): Promise<void> {
  if (!packagePath) throw new Error("Bitbucket plugin package path is required");
  await testPage.goto("/settings/plugins");
  await testPage.getByTestId("install-plugin-trigger").click();
  await testPage.getByTestId("install-plugin-tab-upload").click();
  await testPage.getByTestId("install-plugin-file-input").setInputFiles(path.resolve(packagePath));
  await testPage.getByTestId("install-plugin-upload-submit").click();
  const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
  await expect(pluginRow).toBeVisible({ timeout: 15_000 });
  await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();
}

async function invokePluginAction(
  apiClient: import("../../helpers/api-client").ApiClient,
  key: string,
  workspaceId: string,
): Promise<unknown> {
  const response = await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/actions/${key}`, {
    workspaceId,
  });
  const body = await response.text();
  expect(response.status, body).toBe(200);
  return JSON.parse(body) as unknown;
}

test.describe("Bitbucket packaged plugin", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("installs the real package through its unconfigured desktop lifecycle", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(90_000);
    const capture = new PrAssetCapture(testPage, testInfo.file, { captureKey: "desktop" });

    await installPackagedPlugin(testPage);

    // These are real RPC calls into the uploaded artifact, not fixture-provider
    // coverage. An empty connection must stay safe and useful without requiring
    // a live Bitbucket account or test credential.
    await expect(
      invokePluginAction(apiClient, "connection.get", seedData.workspaceId),
    ).resolves.toEqual({
      state: "unconfigured",
      healthy: false,
    });
    await expect(
      invokePluginAction(apiClient, "repositories.list", seedData.workspaceId),
    ).resolves.toEqual({
      repositories: [],
    });
    await expect(
      invokePluginAction(apiClient, "pullrequests.queue", seedData.workspaceId),
    ).resolves.toEqual({
      pull_requests: [],
    });

    await mockConfiguredCaptureState(testPage);
    await testPage.setViewportSize({ width: 1440, height: 900 });
    await testPage.goto("/bitbucket");
    const workbench = testPage.getByTestId("bitbucket-workbench");
    await expect(workbench).toBeVisible();
    await expect(workbench.getByTestId("bitbucket-connection-health")).toHaveCount(0);
    if (captureEnabled) {
      await expect(workbench.getByTestId("bitbucket-scope-bar")).toBeVisible();
      await expect(workbench.getByTestId("bitbucket-list-toolbar")).toBeVisible();
      await expect(workbench.getByTestId("bitbucket-pr-queue")).toBeVisible();
      await expect(
        workbench.getByText("Add audit log export with retention controls"),
      ).toBeVisible();
    } else {
      await expect(workbench.getByText("Bitbucket needs attention")).toBeVisible();
      await expect(workbench.getByRole("button", { name: "Configure Bitbucket" })).toBeVisible();
    }
    await capture.screenshot("desktop-bitbucket-workbench", {
      caption: "Desktop Bitbucket pull-request workbench using native Kandev controls",
    });

    // Deactivation must revoke the actual package's navigation/runtime entries;
    // enabling it loads the uploaded package again, not the fixture contract.
    await testPage.goto("/settings/plugins");
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await pluginRow.getByRole("button", { name: "Disable" }).click();
    await expect(pluginRow.getByText("Disabled", { exact: true })).toBeVisible();
    await testPage.goto("/bitbucket");
    await expect(testPage.getByTestId("bitbucket-workbench")).toHaveCount(0);
    await testPage.goto("/settings/plugins");
    await pluginRow.getByRole("button", { name: "Enable" }).click();
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();
    await testPage.goto("/bitbucket");
    await expect(testPage.getByTestId("bitbucket-workbench")).toBeVisible();
    capture.flush();
  });

  test("renders the uploaded package's mobile workbench in a touch-sized viewport", async ({
    testPage,
  }, testInfo) => {
    test.setTimeout(90_000);
    const capture = new PrAssetCapture(testPage, testInfo.file, { captureKey: "mobile" });

    await installPackagedPlugin(testPage);

    await mockConfiguredCaptureState(testPage);
    await testPage.setViewportSize({ width: 393, height: 851 });
    await testPage.goto("/bitbucket");
    const workbench = testPage.getByTestId("bitbucket-workbench");
    await expect(workbench).toHaveClass(/bb-mobile/);
    if (captureEnabled) {
      const filterButton = testPage.getByRole("button", { name: "Open Bitbucket filters" });
      await expect(filterButton).toBeVisible();
      expect((await filterButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      await filterButton.click();
      await expect(testPage.getByRole("heading", { name: "Bitbucket filters" })).toBeVisible();
      await waitForFiniteAnimations(testPage.locator("body"));
      await expectContainedInViewport(testPage, ".bb-filter-sheet");
    } else {
      const configureButton = workbench.getByRole("button", { name: "Configure Bitbucket" });
      await expect(configureButton).toBeVisible();
      expect((await configureButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    }
    await expectNoHorizontalOverflow(testPage);
    await capture.screenshot("mobile-bitbucket-filters", {
      caption: "Native mobile Bitbucket filter sheet",
    });
    capture.flush();
  });

  test("shows the shared pull-request summary from the uploaded package on sidebar hover", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const title = "Bitbucket sidebar summary";
    const task = await apiClient.createTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const getDetailRequests = await mockLinkedPullRequestState(testPage, task.id);

    await installPackagedPlugin(testPage);
    await testPage.goto("/");

    const taskRow = testPage.getByTestId("sidebar-task-item").filter({ hasText: title });
    await expect(taskRow).toBeVisible({ timeout: 15_000 });
    const icon = taskRow.getByTestId(`registered-change-request-task-icon-${task.id}`);
    await expect(icon).toHaveAttribute("aria-label", "1 Bitbucket pull request linked");
    await expect.poll(getDetailRequests).toBeGreaterThan(0);
    await expect(icon).toHaveClass(/text-green-500/);

    await icon.hover();
    const summary = testPage
      .locator(
        '[data-slot="tooltip-content"]:not([data-state="closed"]) > [data-testid="pr-task-status-summary"]',
      )
      .first();
    await expect(summary).toBeVisible();
    await expect(summary.getByTestId("pr-task-status-number")).toHaveText("PR #42");
    await expect(summary.getByTestId("pr-task-status-title")).toHaveText(
      "Add audit log export with retention controls",
    );
    await expect(summary.getByTestId("pr-task-status-review")).toContainText("Approved");
    await expect(summary.getByTestId("pr-task-status-ci")).toContainText("Passed");
  });
});
