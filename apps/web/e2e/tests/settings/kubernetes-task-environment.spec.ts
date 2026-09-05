import path from "node:path";
import type { Locator, Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { seedKubernetesTaskEnvironment } from "./kubernetes-task-environment-helpers";

const INITIAL_POD_NAME = "kandev-desktop-task-pod";
const POD_TEMPLATE =
  "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n      - name: kandev-agent\n        image: ghcr.io/kdlbs/kandev:latest\n";

function openTooltip(page: Page): Locator {
  return page.getByRole("tooltip").filter({ hasText: INITIAL_POD_NAME }).last();
}

function statusTrigger(icon: Locator): Locator {
  return icon.locator("..");
}

async function expectEagerStatusRead(reads: () => number, baseline: number) {
  await expect.poll(reads).toBeGreaterThan(baseline);
}

async function expectStatusFailure(icon: Locator, page: Page) {
  const trigger = statusTrigger(icon);
  await expect(trigger).toHaveClass(/text-destructive/);
  await expect(icon).not.toHaveClass(/text-muted-foreground/);
  await trigger.hover();
  const tooltip = openTooltip(page);
  await expect(tooltip.getByTestId("remote-executor-status-summary")).toBeVisible();
  await expect(tooltip.getByTestId("remote-executor-status-identity")).toHaveText(INITIAL_POD_NAME);
  await expect(tooltip.getByTestId("remote-executor-status-error")).toContainText("Unauthorized");
}

async function expectStatusHealthy(icon: Locator, page: Page) {
  const trigger = statusTrigger(icon);
  await expect(trigger).toHaveClass(/text-emerald-500/);
  await expect(icon).not.toHaveClass(/text-muted-foreground/);
  await trigger.hover();
  const tooltip = openTooltip(page);
  await expect(tooltip.getByTestId("remote-executor-status-summary")).toBeVisible();
  await expect(tooltip.getByTestId("remote-executor-status-identity")).toHaveText(INITIAL_POD_NAME);
  await expect(tooltip.getByTestId("remote-executor-status-state")).toContainText("running");
  await expect(tooltip.getByTestId("remote-executor-status-restarts")).toContainText("3");
  await expect(tooltip.getByTestId("remote-executor-status-error")).toHaveCount(0);
}

test("Kubernetes task icons hydrate eagerly and publish the structured Pod summary", async ({
  apiClient,
  backend,
  seedData,
  testPage,
}) => {
  const executor = await apiClient.createExecutor("Desktop task Kubernetes", "k8s", {
    auth_mode: "in_cluster",
    kubeconfig_path: "",
    namespace: "default",
    request_timeout_seconds: "1",
  });
  const profile = await apiClient.createExecutorProfile(executor.id, {
    name: "Desktop task Kubernetes profile",
    config: {
      platform: "linux/amd64",
      main_container: "kandev-agent",
      pod_template_yaml: POD_TEMPLATE,
      "workspace.mode": "empty_dir",
    },
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });
  let taskId = "";
  let sessionReads = 0;
  let environmentReads = 0;
  let statusUnauthorized = true;
  let deferNextSessionRead = false;
  let releaseSessionRead: () => void = () => undefined;
  let markDeferredReadEntered: () => void = () => undefined;
  const deferredReadEntered = new Promise<void>((resolve) => {
    markDeferredReadEntered = resolve;
  });
  const deferredSessionRelease = new Promise<void>((resolve) => {
    releaseSessionRead = resolve;
  });

  try {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Desktop Kubernetes task disclosure",
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

    testPage.on("request", (request) => {
      if (request.url().includes(`/api/v1/tasks/${task.id}/environment/live`)) {
        environmentReads += 1;
      }
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
        if (deferNextSessionRead) {
          deferNextSessionRead = false;
          markDeferredReadEntered();
          await deferredSessionRelease;
        }
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify([
            {
              task_id: task.id,
              session_id: sessionId,
              pod_name: INITIAL_POD_NAME,
              pod_phase: "Running",
              container_state: "running",
              restarts: sessionReads > 1 ? 3 : 0,
              workspace_kind: "empty_dir",
              created_at: "2026-08-31T10:00:00Z",
              failure_reason: statusUnauthorized ? "Unauthorized" : undefined,
            },
          ]),
        });
      },
    );

    const kanban = new KanbanPage(testPage);
    let readsBeforeRender = sessionReads;
    await kanban.goto();
    const kanbanIcon = kanban.taskCard(task.id).getByTestId("executor-status-kubernetes-icon");
    await expectEagerStatusRead(() => sessionReads, readsBeforeRender);
    await expectStatusFailure(kanbanIcon, testPage);
    statusUnauthorized = false;
    readsBeforeRender = sessionReads;
    await testPage.reload();
    await expectEagerStatusRead(() => sessionReads, readsBeforeRender);
    await expectStatusHealthy(
      kanban.taskCard(task.id).getByTestId("executor-status-kubernetes-icon"),
      testPage,
    );

    statusUnauthorized = true;
    readsBeforeRender = sessionReads;
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad(30_000);
    await expectEagerStatusRead(() => sessionReads, readsBeforeRender);
    await expectStatusFailure(
      session.sidebar
        .locator(`[data-task-row-id="${task.id}"]`)
        .getByTestId("executor-status-kubernetes-icon"),
      testPage,
    );
    statusUnauthorized = false;
    readsBeforeRender = sessionReads;
    await testPage.reload();
    await session.waitForLoad(30_000);
    await expectEagerStatusRead(() => sessionReads, readsBeforeRender);
    await expectStatusHealthy(
      session.sidebar
        .locator(`[data-task-row-id="${task.id}"]`)
        .getByTestId("executor-status-kubernetes-icon"),
      testPage,
    );
    const trigger = testPage.getByTestId("executor-settings-button");
    await expect(trigger).toBeVisible();
    deferNextSessionRead = true;
    await trigger.hover();

    const disclosure = testPage.getByTestId("executor-settings-popover");
    await expect(disclosure).toBeVisible();
    await expect(disclosure.getByTestId("kubernetes-environment-summary")).toBeVisible();
    await expect(disclosure.locator("svg.tabler-icon-package")).toBeVisible();
    await expect(disclosure).toContainText(INITIAL_POD_NAME);
    await deferredReadEntered;

    const refresh = disclosure.getByTestId("executor-settings-refresh");
    const readsBeforeClick = { environmentReads, sessionReads };
    await refresh.click();
    await expect(refresh).toHaveAttribute("aria-busy", "true");
    await expect(disclosure.getByTestId("executor-settings-refresh-spinner")).toBeVisible();
    expect(environmentReads).toBe(readsBeforeClick.environmentReads);
    expect(sessionReads).toBe(readsBeforeClick.sessionReads);

    releaseSessionRead();
    await expect(refresh).toHaveAttribute("aria-busy", "false");
    await expect(disclosure.getByTestId("kubernetes-restart-count")).toHaveText("3");
    await expect(disclosure.getByRole("button", { name: "Copy Pod" })).toBeVisible();
    await expect(disclosure.getByTestId("executor-settings-reset")).toHaveCount(0);
    await expect(disclosure.getByTestId("executor-settings-link")).toHaveAttribute(
      "href",
      `/settings/executors/${profile.id}`,
    );
  } finally {
    if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
    await apiClient.deleteExecutorProfile(profile.id).catch(() => undefined);
    await apiClient.deleteExecutor(executor.id).catch(() => undefined);
  }
});
