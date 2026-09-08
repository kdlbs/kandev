import path from "node:path";
import type { Locator } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import { seedKubernetesTaskEnvironment } from "./kubernetes-task-environment-helpers";

const POD_NAME = "kandev-mobile-task-pod";

async function expectTouchLocator(locator: Locator, label: string) {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  expect(box, `${label} must have geometry`).not.toBeNull();
  expect(box!.height, `${label} must be at least 44px tall`).toBeGreaterThanOrEqual(44);
}

async function expectExpandedTouchTarget(locator: Locator, label: string) {
  await expect(locator).toBeVisible();
  const size = await locator.evaluate((element) => {
    const style = getComputedStyle(element, "::after");
    return { height: Number.parseFloat(style.height), width: Number.parseFloat(style.width) };
  });
  expect(size.height, `${label} must be at least 44px tall`).toBeGreaterThanOrEqual(44);
  expect(size.width, `${label} must be at least 44px wide`).toBeGreaterThanOrEqual(44);
}

test("Kubernetes task disclosure exposes live Pod details and safe actions by touch", async ({
  apiClient,
  backend,
  seedData,
  testPage,
}) => {
  const executor = await apiClient.createExecutor("Mobile task Kubernetes", "k8s", {
    auth_mode: "in_cluster",
    kubeconfig_path: "",
    namespace: "default",
    request_timeout_seconds: "1",
  });
  const profile = await apiClient.createExecutorProfile(executor.id, {
    name: "Mobile task Kubernetes profile",
    config: {
      platform: "linux/amd64",
      main_container: "kandev-agent",
      pod_template_yaml:
        "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n      - name: kandev-agent\n        image: ghcr.io/kdlbs/kandev:latest\n",
      "workspace.mode": "empty_dir",
    },
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });
  let taskId = "";
  let navigationTaskId = "";
  let sessionReads = 0;

  try {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Kubernetes task disclosure",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        executor_id: executor.id,
        executor_profile_id: profile.id,
      },
    );
    taskId = task.id;
    expect(task.session_id).toBeTruthy();
    const sessionId = task.session_id!;
    seedKubernetesTaskEnvironment(
      path.join(backend.tmpDir, "kandev.db"),
      task.id,
      sessionId,
      executor.id,
      profile.id,
    );
    const navigationTask = await apiClient.seedTask(
      seedData.workspaceId,
      "Mobile executor disclosure navigation",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    navigationTaskId = navigationTask.task_id;
    await apiClient.seedTaskSession(navigationTaskId, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });

    await testPage.route(
      `**/api/v1/kubernetes/executors/${executor.id}/sessions?*`,
      async (route) => {
        const query = new URL(route.request().url()).searchParams;
        if (query.get("task_id") !== task.id || query.get("session_id") !== sessionId) {
          await route.continue();
          return;
        }
        sessionReads += 1;
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify([
            {
              task_id: task.id,
              session_id: sessionId,
              pod_name: POD_NAME,
              pod_phase: "Running",
              container_state: "running",
              restarts: 0,
              workspace_kind: "empty_dir",
              created_at: "2026-08-25T10:00:00Z",
            },
          ]),
        });
      },
    );

    await testPage.goto(`/t/${navigationTaskId}`);
    await new SessionPage(testPage).waitForLoad(30_000);

    await testPage.getByTestId("mobile-session-menu").tap();
    const taskSwitcher = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = taskSwitcher.locator(`[data-task-row-id="${task.id}"]`);
    const statusTrigger = taskRow.getByTestId("remote-executor-status-trigger");
    await expect.poll(() => sessionReads).toBeGreaterThan(0);
    await expect(statusTrigger).toHaveClass(/text-emerald-500/);
    await expect(statusTrigger).toHaveAttribute("aria-haspopup", "dialog");
    await expectExpandedTouchTarget(statusTrigger, "Mobile task executor status action");
    await statusTrigger.tap();

    const statusDrawer = testPage.getByTestId("remote-executor-status-drawer");
    await expect(statusDrawer).toBeVisible();
    await expect(statusDrawer.getByTestId("remote-executor-status-summary")).toBeVisible();
    await expect(statusDrawer.getByTestId("remote-executor-status-identity")).toHaveText(POD_NAME);
    await expect(statusDrawer.getByTestId("remote-executor-status-state")).toContainText("running");
    await expect(statusDrawer.getByTestId("remote-executor-status-restarts")).toContainText("0");
    await expect(testPage).toHaveURL(new RegExp(`/t/${navigationTaskId}$`));
    await statusDrawer.getByRole("button", { name: "Close" }).tap();
    await expect(statusDrawer).toBeHidden();
    await expect(testPage).toHaveURL(new RegExp(`/t/${navigationTaskId}$`));
    await testPage.goto(`/t/${task.id}`);
    await new SessionPage(testPage).waitForLoad(30_000);

    const trigger = testPage.getByTestId("executor-settings-button");
    await expect(trigger).toBeVisible();
    await expect(trigger).toHaveAccessibleName(/executor settings/i);
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
    expect(triggerBox!.width).toBeGreaterThanOrEqual(44);

    await trigger.tap();
    const drawer = testPage.getByTestId("executor-settings-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByTestId("kubernetes-environment-summary")).toBeVisible();
    await expect(drawer.locator("svg.tabler-icon-package")).toBeVisible();
    await expect(drawer).toContainText(POD_NAME);
    await expect(drawer).toContainText("Running");
    await expect(drawer).toContainText("empty_dir");
    await expect(drawer).not.toContainText("No resource details available.");
    await expect(drawer.getByTestId("kubernetes-restart-count")).toHaveText("0");
    await expectTouchLocator(
      drawer.getByRole("button", { name: "Copy Pod" }),
      "Mobile Pod copy button",
    );
    await expectTouchLocator(
      drawer.getByTestId("executor-settings-refresh"),
      "Mobile Kubernetes refresh action",
    );
    await expect(drawer.getByTestId("executor-settings-reset")).toHaveCount(0);
    await expect(drawer.getByTestId("executor-settings-link")).toHaveAttribute(
      "href",
      `/settings/executors/${profile.id}`,
    );
    await expectTouchLocator(
      drawer.getByTestId("executor-settings-link"),
      "Mobile Kubernetes settings action",
    );
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Kubernetes task disclosure");
  } finally {
    if (navigationTaskId) await apiClient.deleteTask(navigationTaskId).catch(() => undefined);
    if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
    await apiClient.deleteExecutorProfile(profile.id).catch(() => undefined);
    await apiClient.deleteExecutor(executor.id).catch(() => undefined);
  }
});
