import path from "node:path";
import { DatabaseSync } from "./node-sqlite";
import { test, expect } from "../fixtures/test-base";
import { waitForSessionDone, waitForAgentMessage } from "./session";
import { SessionPage } from "../pages/session-page";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import type { ExecutorFailureEpisode } from "../../lib/types/executor-failure";

function seedEpisode(database: string, taskId: string): ExecutorFailureEpisode {
  const db = new DatabaseSync(database);
  const now = new Date().toISOString();
  const episode: ExecutorFailureEpisode = {
    id: `failure-${taskId}`,
    task_id: taskId,
    environment_id: `fixture-${taskId}`,
    revision: 1,
    state: "active",
    current_outcome: "terminated",
    first_observed_at: now,
    last_observed_at: now,
    observation: {
      outcome: "terminated",
      runtime: "k8s",
      observed_at: now,
      reason: "Evicted",
      message: 'Usage of EmptyDir volume "docker-data" exceeds the limit "12Gi".',
      pod_phase: "Failed",
      container_ready: false,
      workspace: "retained",
      containers: [
        { name: "kandev-agent", state: "terminated", ready: false, exit_code: 137 },
        { name: "docker", state: "terminated", ready: false, exit_code: 137 },
      ],
    },
  };
  try {
    db.exec("PRAGMA busy_timeout=10000");
    db.prepare(
      `INSERT INTO executor_failure_episodes(id,task_id,environment_id,ownership_generation,resource_key,revision,state,current_outcome,first_observed_at,last_observed_at,evidence) VALUES(?,?,?,1,'synthetic-e2e-resource',1,'active','terminated',?,?,?)`,
    ).run(
      episode.id,
      taskId,
      episode.environment_id!,
      now,
      now,
      JSON.stringify(episode.observation),
    );
  } finally {
    db.close();
  }
  return episode;
}

export function executorFailureVisibilityScenario() {
  test("executor cause survives reload and recovery distinguishes fresh conversation", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Executor failure evidence",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    expect(task.session_id).toBeTruthy();
    await waitForSessionDone(apiClient, task.id, task.session_id!, "fixture completed");
    const episode = seedEpisode(path.join(backend.tmpDir, "kandev.db"), task.id);
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${task.id}`);
    await session.waitForLoad();
    const strip = testPage.getByTestId("session-executor-failure-card");
    await test.step("durable failure strip on task load", async () => {
      const summary = await apiClient.listTasks(seedData.workspaceId);
      const current = summary.tasks.find((candidate) => candidate.id === task.id);
      expect(current?.status_summary).toMatchObject({ executor_failure: { id: episode.id } });
      await expect(strip).toContainText("Executor evicted");
    });
    await testPage.reload();
    await session.waitForLoad();
    await expect(strip).toContainText("Executor evicted");
    const collapsed = await strip.boundingBox();
    expect(collapsed).not.toBeNull();
    expect(collapsed!.y).toBeGreaterThan(testPage.viewportSize()!.height / 2);
    const opener = testPage.getByTestId("executor-failure-expand");
    await opener.click();
    const details = testPage.getByTestId("executor-failure-details");
    await expect(strip).toContainText("12Gi");
    await expect(testPage.getByTestId("task-shared-error")).toHaveCount(0);
    await expect(testPage.getByRole("dialog")).toHaveCount(0);
    await expect(details).toContainText("persistent volume");
    await expect(details).toContainText("conversation recovery is unverified");
    await expect(details).not.toContainText("out of memory");
    await expect(details.getByRole("button", { name: /resume|reset/i })).toHaveCount(0);
    await testPage.getByTestId("executor-recheck").click();
    await expect(strip.getByRole("status")).toContainText("Cannot verify");
    await expect(strip).toContainText("Executor evicted");
    await assertNoDocumentHorizontalOverflow(testPage);
    if (testInfo.project.name === "mobile-chrome") {
      const box = await strip.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
      expect(box!.height).toBeLessThanOrEqual(testPage.viewportSize()!.height / 2 + 1);
      const recheckBox = await testPage.getByTestId("executor-recheck").boundingBox();
      expect(recheckBox!.height).toBeGreaterThanOrEqual(44);
    }
    await testPage.route(`**/api/v1/tasks/${task.id}/executor-failure/recheck`, async (route) => {
      const body = route.request().postDataJSON();
      expect(body).toEqual({ episode_id: episode.id, revision: 1 });
      const db = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
      try {
        db.exec("PRAGMA busy_timeout=10000");
        db.prepare(
          "UPDATE executor_failure_episodes SET state='resolved',current_outcome='healthy',revision=2,resolved_at=? WHERE id=?",
        ).run(new Date().toISOString(), episode.id);
      } finally {
        db.close();
      }
      await route.fulfill({
        json: { ...episode, state: "resolved", revision: 2, current_outcome: "healthy" },
      });
    });
    await testPage.getByTestId("executor-recheck").click();
    await expect(strip).toHaveCount(0);
    await apiClient.seedSessionMessage(task.session_id!, {
      type: "status",
      content: "",
      metadata: {
        executor_recovery: true,
        provider_conversation: "fresh",
        workspace: "retained",
        workspace_observed_at: episode.observation.observed_at,
      },
    });
    await testPage.reload();
    await session.waitForLoad();
    await expect(strip).toHaveCount(0);
    const history = testPage.getByTestId("executor-recovery-history");
    await expect(history).toHaveAttribute("role", "alert");
    await expect(history.getByRole("heading")).toContainText("Executor recovery confirmed");
    await expect(history).toContainText("A new provider conversation was started");
    await expect(testPage.getByTestId("executor-recovery-history")).toContainText(
      "persistent volume was retained",
    );
    await expect(testPage.getByTestId("executor-recovery-history")).toContainText(
      "Executor recovery confirmed",
    );
    await assertNoDocumentHorizontalOverflow(testPage);
  });
  // @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.2, .6, .13
  test("worker outage retains uncertainty and separate workspace guidance after reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    let heldSessionId: string | undefined;
    const { settings } = await apiClient.getUserSettings();
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Worker connectivity warning",
        seedData.agentProfileId,
        {
          description: "/e2e:cancel-hold executor-outage-ui-accepted",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      heldSessionId = task.session_id!;
      await waitForAgentMessage(apiClient, heldSessionId, "executor-outage-ui-accepted");
      await expect
        .poll(
          async () =>
            (await apiClient.listTaskSessions(task.id)).sessions.find(
              (candidate) => candidate.id === heldSessionId,
            )?.state,
        )
        .toBe("RUNNING");
      const agentReplyIds = async () =>
        (await apiClient.listSessionMessages(heldSessionId!)).messages
          .filter((message) => message.author_type === "agent" && message.type !== "status")
          .map((message) => message.id);
      const originalReplies = await agentReplyIds();
      const episode = seedEpisode(path.join(backend.tmpDir, "kandev.db"), task.id);
      episode.current_outcome = "unknown";
      episode.observation = {
        ...episode.observation,
        outcome: "unknown",
        reason: "WorkerUnavailable",
        message: "",
        container_ready: false,
        pod_conditions: [
          { type: "Ready", status: "False", reason: "NodeNotReady" },
          { type: "DisruptionTarget", status: "True", reason: "DeletionByTaintManager" },
        ],
        containers: [{ name: "kandev-agent", state: "running", ready: true, restarts: 0 }],
        secondary: [
          {
            operation: "cleanup",
            reason: "cleanup_failed",
            occurred_at: episode.observation.observed_at,
          },
        ],
      };
      const db = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
      try {
        db.exec("PRAGMA busy_timeout=10000");
        db.prepare(
          "UPDATE executor_failure_episodes SET current_outcome='unknown',evidence=? WHERE id=?",
        ).run(JSON.stringify(episode.observation), episode.id);
      } finally {
        db.close();
      }
      const session = new SessionPage(testPage);
      await testPage.goto(`/t/${task.id}`);
      await session.waitForLoad();
      const strip = testPage.getByTestId("session-executor-failure-card");
      await expect(strip).toContainText("Executor connection lost");
      await testPage.reload();
      await session.waitForLoad();
      await expect(strip).toContainText("Executor connection lost");
      await testPage.getByTestId("executor-failure-expand").click();
      const details = testPage.getByTestId("executor-failure-details");
      await expect(strip).toContainText("Agent status cannot be verified");
      await expect(strip).toContainText("Restore worker connectivity");
      await expect(details).toContainText("persistent volume was retained");
      await expect(details).toContainText("Workspace access and data integrity are unverified");
      await expect(details).not.toContainText("out of memory");
      await expect(details.getByRole("button", { name: /resume|reset/i })).toHaveCount(0);
      await details.locator("summary").click();
      await expect(details).toContainText("cleanup_failed");
      await expect(details).toContainText("DeletionByTaintManager");
      await testPage.getByTestId("executor-recheck").click();
      await expect(strip.getByRole("status")).toContainText("Cannot verify");
      await expect(strip).toContainText("Executor connection lost");
      expect(
        (await apiClient.listTaskSessions(task.id)).sessions.find(
          (candidate) => candidate.id === heldSessionId,
        )?.state,
      ).toBe("RUNNING");
      expect(await agentReplyIds()).toEqual(originalReplies);
      await assertNoDocumentHorizontalOverflow(testPage);
      if (testInfo.project.name === "mobile-chrome") {
        const recheck = await testPage.getByTestId("executor-recheck").boundingBox();
        expect(recheck!.height).toBeGreaterThanOrEqual(44);
      }
    } finally {
      try {
        if (heldSessionId) await apiClient.stopSession({ session_id: heldSessionId });
      } finally {
        await apiClient.saveUserSettings({
          prevent_auto_start_agent_on_open: settings.prevent_auto_start_agent_on_open,
        });
      }
    }
  });
}
