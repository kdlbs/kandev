import { execFileSync } from "node:child_process";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

const POD_NAME = "kandev-mobile-task-pod";

function seedKubernetesTaskEnvironment(
  database: string,
  taskId: string,
  sessionId: string,
  executorId: string,
  profileId: string,
) {
  const values = [taskId, sessionId, executorId, profileId];
  if (values.some((value) => !/^[a-zA-Z0-9-]+$/.test(value))) {
    throw new Error("test fixture identities must be alphanumeric UUID-like values");
  }
  const environmentId = `environment-${taskId}`;
  execFileSync("sqlite3", [
    database,
    `INSERT INTO task_environments (
       id, task_id, executor_type, executor_id, executor_profile_id,
       control_port, status, workspace_path, container_id, sandbox_id,
       task_dir_name, created_at, updated_at
     ) VALUES (
       '${environmentId}', '${taskId}', 'k8s', '${executorId}', '${profileId}',
       8765, 'ready', '', '', '', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
     );
     UPDATE task_sessions SET task_environment_id = '${environmentId}' WHERE id = '${sessionId}';`,
  ]);
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

    await testPage.route(
      `**/api/v1/kubernetes/executors/${executor.id}/sessions?*`,
      async (route) => {
        const query = new URL(route.request().url()).searchParams;
        if (query.get("task_id") !== task.id || query.get("session_id") !== sessionId) {
          await route.continue();
          return;
        }
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
    await expect(drawer).toContainText(POD_NAME);
    await expect(drawer).toContainText("Running");
    await expect(drawer).toContainText("empty_dir");
    await expect(drawer).not.toContainText("No resource details available.");
    await expect(drawer.getByTestId("executor-settings-refresh")).toBeVisible();
    await expect(drawer.getByTestId("executor-settings-reset")).toHaveCount(0);
    await expect(drawer.getByTestId("executor-settings-link")).toHaveAttribute(
      "href",
      `/settings/executors/k8s/${executor.id}`,
    );
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Kubernetes task disclosure");
  } finally {
    if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
    await apiClient.deleteExecutorProfile(profile.id).catch(() => undefined);
    await apiClient.deleteExecutor(executor.id).catch(() => undefined);
  }
});
